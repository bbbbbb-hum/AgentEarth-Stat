package jobs

import (
	"AgentEarth-Stat/cron/internal/fund"
	"AgentEarth-Stat/cron/internal/svc"
	fundmodel "AgentEarth-Stat/models/fund"
	"context"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// SettlementJob 日结核销：按 FEFO 将 ae_user_consumption_record_daily 的消费分摊到充值批次，落库 ae_recharge_allocation
// 采用"代数和法":允许旧充值透支，透支后用户余额统计结果为负值，新充值记录先去填补余额的负值，再去进行消费核销
type SettlementJob struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSettlementJob(ctx context.Context, svcCtx *svc.ServiceContext) *SettlementJob {
	return &SettlementJob{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (j *SettlementJob) Run() {
	// 处理昨日消费
	targetDate := time.Now().AddDate(0, 0, -1)
	j.Infof("[SettlementJob] 开始核销日期 %s 的消费记录", targetDate.Format("2006-01-02"))
	if err := j.settleAllUsersConsumption(targetDate); err != nil {
		j.Errorf("[SettlementJob] 核销失败: %v", err)
	}
}

func (j *SettlementJob) settleAllUsersConsumption(targetDate time.Time) error {
	// 查询昨日所有的消费记录
	dateStr := targetDate.Format("2006-01-02")
	dailyRecords, err := j.svcCtx.UserConsumptionRecordDailyModel.QueryDailyConsumptionByDay(j.ctx, dateStr)
	if err != nil {
		j.Errorf("[SettlementJob] 查询日消费记录失败: %v", err)
		return err
	}
	if len(dailyRecords) == 0 {
		j.Infof("[SettlementJob] 日期 %s 未发现任何消费记录", dateStr)
		return nil
	}
	j.Infof("[SettlementJob] 共 %d 条用户日消费待核销", len(dailyRecords))
	var totalNewAllocs int64 // 记录最终插入了多少条核销记录
	for i := range dailyRecords {
		n, procErr := j.processUserDailyConsumption(&dailyRecords[i]) // 遍历每条消费记录去做核销
		if procErr != nil {
			j.Errorf("[SettlementJob] 用户 %s 核销失败: %v", dailyRecords[i].UserId, procErr)
			continue
		}
		totalNewAllocs += n
	}
	j.Infof("[SettlementJob] 日期 %s 核销完成，实际新增 %d 条核销", dateStr, totalNewAllocs)
	return nil
}

func (j *SettlementJob) processUserDailyConsumption(daily *fundmodel.DailyRecordRow) (int64, error) {
	// consumeAmount 表示“消费总额”（固定不变）；amountToDeduct 表示“剩余待扣”（会逐步减少）
	consumeAmount := decimal.NewFromFloat(daily.XlcreditConsume)
	if consumeAmount.LessThanOrEqual(decimal.Zero) {
		return 0, nil
	}
	amountToDeduct := consumeAmount

	// 幂等检查：已完全分摊则跳过，查询已核销总额，如果>=消费总额，说明已处理过，直接跳过
	allocatedSumStr, err := j.svcCtx.RechargeAllocationModel.GetAllocatedSumByConsumptionDailyId(j.ctx, daily.Id)
	if err == nil {
		if allocatedSum, parseErr := decimal.NewFromString(allocatedSumStr); parseErr == nil && allocatedSum.GreaterThanOrEqual(consumeAmount) {
			j.Infof("[SettlementJob] 日消费ID=%d 用户=%s 已完全分摊，跳过", daily.Id, daily.UserId)
			return 0, nil
		}
	}

	var newAllocCount int64
	err = j.svcCtx.DB.TransactCtx(j.ctx, func(ctx context.Context, session sqlx.Session) error {
		txRechargeModel := j.svcCtx.UserRechargeRecordModel.WithSession(session)
		txAllocModel := j.svcCtx.RechargeAllocationModel.WithSession(session)
		txBalanceModel := j.svcCtx.UserBalanceStatisticDailyModel.WithSession(session)

		// 事务内按“已分摊金额”重算剩余待扣，避免部分成功后重跑又从全额开始扣导致过度分摊
		// allocatedSumStr 代表“当前已核销金额”——针对某一条消费记录
		// 查询已分摊金额失败时，直接中断事务，避免退化为“从消费总额重新扣”导致过度核销（避免按照错误金额执行整体核销任务）
		// TODO: 这里应该有报错预警，等待人工处理
		allocatedSumStr, err := txAllocModel.GetAllocatedSumByConsumptionDailyId(ctx, daily.Id)
		if err != nil {
			return err
		}
		if allocatedSum, parseErr := decimal.NewFromString(allocatedSumStr); parseErr == nil {
			amountToDeduct = consumeAmount.Sub(allocatedSum) // 待核销金额 = 消费金额 - 已核销金额
		}
		if amountToDeduct.LessThanOrEqual(decimal.Zero) {
			return nil
		}

		// =========================================================
		// 步骤 A: 计算用户全局实时净资产 (Global Net Balance)
		// =========================================================
		// 优化策略：Snapshot + Delta
		// 1. 获取最近的一条余额统计快照
		// 2. 累加快照日期之后的资金变动（充值 - 核销 - 系统扣减）
		var snapshotBal decimal.Decimal
		var deltaStartTime time.Time // 增量计算的起始时间

		// 1. 查快照（统计表 ae_user_balance_statistic_daily）
		snap, err := txBalanceModel.QueryLatestBalanceSnapshot(ctx, daily.UserId)
		if err != nil {
			return err
		}
		if snap != nil {
			// 找到快照
			if val, err := decimal.NewFromString(snap.Balance); err == nil {
				snapshotBal = val
			}
			// 增量计算从快照日期的第二天 00:00:00 开始
			// 例如快照是 2025-02-04，说明统计了截止 04 日 23:59:59 的数据，那么增量应该算 2025-02-05 00:00:00 之后的数据
			deltaStartTime = snap.Day.AddDate(0, 0, 1)
		}
		// 新用户或无快照时，deltaStartTime 为零值，SQL 中会匹配所有时间（从盘古开天地开始算）

		// 2. 算增量 (Delta)，只统计 deltaStartTime 之后的变动
		// SQL 含义：1. 正向充值记录 2. 减去新增消费核销（重试场景下把“上次失败遗留的扣款”纳入总账）3. 减去负向充值记录
		deltaStr, err := txRechargeModel.QueryBalanceDeltaSince(ctx, daily.UserId, deltaStartTime)
		if err != nil {
			return err
		}
		var deltaBal decimal.Decimal
		if val, err := decimal.NewFromString(deltaStr); err == nil {
			deltaBal = val
		}

		// 3. 全局余额 = 快照 + 增量；全局余额代表今日还未扣减消费金额时的可用余额
		globalBalance := snapshotBal.Add(deltaBal)

		// 查候选充值记录：FEFO 排序
		// 业务语义：只要在消费发生那一天内还未过期的批次，都可以用于结算这一天的消费；
		// 因此按“消费日的下一天零点”作为有效期边界，而不是按当前 time.Now()。
		// 只有在整天（如 2 月 4 日 00:00–24:00）都没有过期的充值批次，才允许用来结算 2 月 4 日的消费。
		// 例如：批次 A expire_time = 2025-02-05 00:30:00，用户在 2 月 4 日白天消费 100，定时核销在 2 月 5 日 01:00 跑；
		// 在 2 月 4 日整天内该批次未过期，但 2 月 5 日 01:00 结算时已过期，会被当成“已过期，不可用”。
		// 核销时按用户消费当天未过期为基准，而不是以当前核销时间过期为基准。
		dayEnd := daily.Day.AddDate(0, 0, 1) // 消费日的次日 00:00:00
		candidates, err := txRechargeModel.QueryRechargeCandidatesForSettlement(ctx, daily.UserId, dayEnd)
		if err != nil {
			return err
		}

		// 【核心修复】如果没有有效记录，去捞取最近一条充值记录（不管过期、是否有钱、是否负数），保证欠款有地方挂，而不是直接忽略
		if len(candidates) == 0 {
			fallbackRec, err := txRechargeModel.QueryFallbackRechargeRecord(ctx, daily.UserId)
			if err != nil {
				return err
			}
			if fallbackRec == nil {
				// 真的没有任何记录（用户从未充值过却产生了消费？），属于严重的业务异常
				j.Errorf("[SettlementJob] 严重异常：用户 %s 没有任何历史充值记录，无法挂载消费 %s", daily.UserId, consumeAmount.String())
				return nil
			}
			j.Infof("[SettlementJob] 用户 %s 无有效充值记录，兜底使用历史记录(ID=%d)进行挂账", daily.UserId, fallbackRec.Id)
			candidates = append(candidates, *fallbackRec)
		}

		// TODO【仅用某条充值记录计算透支量有局限性】
		// 代数和法：可先扫一遍看用户之前欠了多少钱，再在扣款时用新批次填补旧批次透支；当前实现依赖“最后一条死磕”与全局余额有效余额逻辑。

		// B. 执行扣款
		for i, rec := range candidates {
			// 如果待核销金额是 0，提前结束
			if amountToDeduct.LessThanOrEqual(decimal.Zero) {
				break
			}

			// 1. 获取物理余额
			initialAmount := decimal.NewFromFloat(rec.XlcreditAmount)
			balance, err := fund.CalculateRealTimeBalance(ctx, txRechargeModel, rec.Id, initialAmount)
			if err != nil {
				j.Errorf("[SettlementJob] 计算批次 %d 余额失败: %v", rec.Id, err)
				return err
				// TODO: 所有计算失败之后应该做什么
			}

			// 判断是否是最后一条记录
			isLastRecord := (i == len(candidates)-1)

			// 2. 如果当前记录已经是负的（已经透支过），直接跳过
			// 它没资格参与扣款；只有当它不是最后一条记录时才跳过，如果是最后一条，即使已透支也要继续被消费记录映射。
			// 场景：昨天 A 透支变为 -50（当时是最后一条）；今天充了 B，A 变成倒数第二条，遍历到 A 时因其已透支且非最后一条则跳过，扣 B。
			if !isLastRecord && balance.LessThanOrEqual(decimal.Zero) {
				continue
			}

			// 2. 计算“有效余额”(Effective Balance)：单条记录的购买力 = Min(物理余额, 全局剩余总资产)
			var effectiveBalance decimal.Decimal
			// --- 分支 A：全局都已经欠费了 ---
			if globalBalance.LessThanOrEqual(decimal.Zero) {
				// 用户不仅没钱还欠平台钱时，当前记录在逻辑上已被拿去填补窟窿，有效余额为 0
				effectiveBalance = decimal.Zero
			} else {
				// --- 分支 B：全局还有钱 ---
				if balance.LessThan(globalBalance) {
					// 子分支 B1：物理余额 < 全局余额，记录的钱是“干净的”，全都能用
					effectiveBalance = balance
				} else {
					// 子分支 B2：物理余额 >= 全局余额，其中一部分是“虚”的已拿去填历史烂账，有效金额被全局水位限制
					effectiveBalance = globalBalance
				}
			}

			// 3. 决定本次扣多少
			var actualDeduct decimal.Decimal
			if isLastRecord {
				// 【死磕逻辑】最后一条不管有效余额够不够，必须扛下所有剩余消费，哪怕透支
				actualDeduct = amountToDeduct
			} else {
				// 【正常逻辑】中间的充值记录只能扣“有效”部分，不能透支
				if effectiveBalance.GreaterThanOrEqual(amountToDeduct) {
					actualDeduct = amountToDeduct
				} else {
					actualDeduct = effectiveBalance
				}
			}

			// 5. 执行落库 (Insert Only)：只有当 actualDeduct 为正数时才插入，保证核销表金额有效
			// 保证幂等性：同一 (consumption_daily_id, recharge_record_id) 只插入一次，冲突时 DO NOTHING
			if actualDeduct.GreaterThan(decimal.Zero) {
				now := time.Now()
				rowsAffected, err := txAllocModel.InsertAllocation(ctx, daily.Id, rec.Id, actualDeduct, now, now)
				if err != nil {
					return err
				}
				if rowsAffected == 0 {
					// 已存在分摊记录（ON CONFLICT DO NOTHING），仍需扣减内存中的剩余待核销额，否则重试时会误报「仍有剩余」
					existingDeductStr, qErr := txAllocModel.GetExistingDeductedAmount(ctx, daily.Id, rec.Id)
					if qErr != nil {
						j.Errorf("[SettlementJob] 幂等分支查询已存在金额失败: daily=%d, recharge=%d, err=%v", daily.Id, rec.Id, qErr)
						return qErr
					}
					existingDeduct, _ := decimal.NewFromString(existingDeductStr)
					// 【修复】重跑时用库中已存在的 deducted_amount 扣减，保证 amountToDeduct 与 DB 一致，不用 actualDeduct
					amountToDeduct = amountToDeduct.Sub(existingDeduct)
					j.Info("[SettlementJob] 幂等跳过: 记录(daily=%d, recharge=%d)已存在，已扣减库中金额=%s，剩余待扣=%s", daily.Id, rec.Id, existingDeductStr, amountToDeduct.String())
					continue
				}
				newAllocCount++
				expireTimeStr := "永久有效"
				if rec.ExpireTime.Valid {
					expireTimeStr = rec.ExpireTime.Time.Format(time.RFC3339)
				}
				j.Infof("[SettlementJob] 核销详情: user_id=%s, 扣减金额=%s, 充值批次ID=%d, 批次过期时间=%s, 操作时间=%s, 剩余需扣=%s",
					daily.UserId, actualDeduct.String(), rec.Id, expireTimeStr, now.Format(time.RFC3339), amountToDeduct.Sub(actualDeduct).String())
				// 更新剩余待扣余额
				amountToDeduct = amountToDeduct.Sub(actualDeduct)
			}
		}

		// 6. 最终检查：由于有“最后一条有效充值记录必须承担剩余所有待扣减”，若仍有剩余则属异常
		if amountToDeduct.GreaterThan(decimal.Zero) {
			j.Errorf("[SettlementJob] 用户 %s 日消费 %v 核销异常，逻辑执行完毕仍有剩余: %v", daily.UserId, daily.XlcreditConsume, amountToDeduct)
		}
		return nil
	})
	return newAllocCount, err
}
