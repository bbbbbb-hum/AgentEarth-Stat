package jobs

import (
	"AgentEarth-Stat/cron/internal/fund"
	"AgentEarth-Stat/cron/internal/svc"
	"context"
	"database/sql"
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

type dailyRecord struct {
	Id              int64     `db:"id"`
	UserId          string    `db:"user_id"`
	Day             time.Time `db:"day"`
	XlcreditConsume float64   `db:"xlcredit_consume"`
	CreateTime      time.Time `db:"create_time"`
}

type rechargeRecord struct {
	Id                int64         `db:"id"`
	UserId            string        `db:"user_id"`
	XlcreditAmount    float64       `db:"xlcredit_amount"`
	PayTime           time.Time     `db:"pay_time"`
	ExpireTime        sql.NullTime  `db:"expire_time"`
	RelatedRechargeId sql.NullInt64 `db:"related_recharge_id"`
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
	//查询昨日所有的消费记录
	query := `
		SELECT id, user_id, day, xlcredit_consume, create_time
		FROM ae_user_consumption_record_daily
		WHERE day = $1 AND xlcredit_consume > 0
	`
	dateStr := targetDate.Format("2006-01-02")
	var dailyRecords []dailyRecord
	if err := j.svcCtx.DB.QueryRowsCtx(j.ctx, &dailyRecords, query, dateStr); err != nil {
		j.Errorf("[SettlementJob] 查询日消费记录失败: %v", err)
		return err
	}
	if len(dailyRecords) == 0 {
		j.Infof("[SettlementJob] 日期 %s 未发现任何消费记录", dateStr)
		return nil
	}
	j.Infof("[SettlementJob] 共 %d 条用户日消费待核销", len(dailyRecords))
	var totalNewAllocs int64 //记录最终插入了多少条核销记录
	for _, record := range dailyRecords {
		n, procErr := j.processUserDailyConsumption(&record) //遍历每条消费记录去做核销
		if procErr != nil {
			j.Errorf("[SettlementJob] 用户 %s 核销失败: %v", record.UserId, procErr)
			continue
		}
		totalNewAllocs += n //累加插入多少条核销记录
	}
	j.Infof("[SettlementJob] 日期 %s 核销完成，实际新增 %d 条核销", dateStr, totalNewAllocs)
	return nil
}

func (j *SettlementJob) processUserDailyConsumption(daily *dailyRecord) (int64, error) {
	// consumeAmount 表示“消费总额”（固定不变）；amountToDeduct 表示“剩余待扣”（会逐步减少）
	consumeAmount := decimal.NewFromFloat(daily.XlcreditConsume)
	if consumeAmount.LessThanOrEqual(decimal.Zero) {
		return 0, nil
	}
	amountToDeduct := consumeAmount
	// 幂等检查：已完全分摊则跳过，查询已核销总额，如果>=消费总额，说明已处理过，直接跳过
	var allocatedSumStr string
	if err := j.svcCtx.DB.QueryRowCtx(j.ctx, &allocatedSumStr,
		"SELECT COALESCE(SUM(deducted_amount), 0) FROM ae_recharge_allocation WHERE consumption_daily_id = $1", daily.Id); err == nil {
		if allocatedSum, parseErr := decimal.NewFromString(allocatedSumStr); parseErr == nil && allocatedSum.GreaterThanOrEqual(consumeAmount) {
			j.Infof("[SettlementJob] 日消费ID=%d 用户=%s 已完全分摊，跳过", daily.Id, daily.UserId)
			return 0, nil
		}
	}

	var newAllocCount int64
	err := j.svcCtx.DB.TransactCtx(j.ctx, func(ctx context.Context, session sqlx.Session) error {
		// 事务内按“已分摊金额”重算剩余待扣，避免部分成功后重跑又从全额开始扣导致过度分摊
		// allocatedSumStr代表 “当前已核销金额”--针对某一条消费记录
		var allocatedSumStr string
		if err := session.QueryRowCtx(ctx, &allocatedSumStr,
			"SELECT COALESCE(SUM(deducted_amount), 0) FROM ae_recharge_allocation WHERE consumption_daily_id = $1", daily.Id); err != nil {
			// 查询已分摊金额失败时，直接中断事务，避免退化为“从消费总额重新扣”导致过度核销(避免按照错误金额执行整体核销任务)
			return err
			//TODO:这里应该有报错预警,等待人工处理
		}
		if allocatedSum, parseErr := decimal.NewFromString(allocatedSumStr); parseErr == nil {
			amountToDeduct = consumeAmount.Sub(allocatedSum) //待核销金额 = 消费金额 - 已核销金额
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

		// 1. 查快照
		// 假设统计表名为 ae_user_balance_statistic_daily
		const snapshotQuery = `
			SELECT balance, day
			FROM ae_user_balance_statistic_daily
			WHERE user_id = $1
			ORDER BY day DESC
			LIMIT 1
		`
		var snap struct {
			Balance string    `db:"balance"`
			Day     time.Time `db:"day"`
		}

		err := session.QueryRowCtx(ctx, &snap, snapshotQuery, daily.UserId)
		if err == nil {
			// 找到快照
			if val, err := decimal.NewFromString(snap.Balance); err == nil {
				snapshotBal = val
			}
			// 增量计算从快照日期的第二天 00:00:00 开始
			// 例如快照是 2025-02-04，说明统计了截止 04日 23:59:59 的数据
			// 那么增量应该算 2025-02-05 00:00:00 之后的数据
			deltaStartTime = snap.Day.AddDate(0, 0, 1)
		} else if err == sql.ErrNoRows {
			// 新用户或无快照，从盘古开天地开始算
			snapshotBal = decimal.Zero
			deltaStartTime = time.Time{} // 零值，SQL中会匹配所有时间
		} else {
			return err
		}

		// 2. 算增量 (Delta)
		// 只统计 deltaStartTime 之后的变动
		const deltaQuery = `
			SELECT
			    ---正向充值记录
				(SELECT COALESCE(SUM(xlcredit_amount), 0) FROM ae_user_recharge_record WHERE user_id = $1 AND xlcredit_amount > 0 AND create_time >= $2) 
				    -
				---减去新增消费核销 ---这个计算是为了在重试场景下，把“上次失败遗留的扣款”纳入总账，正常情况下是没有的
				(SELECT COALESCE(SUM(deducted_amount), 0) FROM ae_recharge_allocation alloc JOIN ae_user_recharge_record rec ON alloc.recharge_record_id = rec.id WHERE rec.user_id = $1 AND alloc.create_time >= $2) 
				    -
				---减去负向充值记录
				(SELECT COALESCE(SUM(ABS(xlcredit_amount)), 0) FROM ae_user_recharge_record WHERE user_id = $1 AND xlcredit_amount < 0 AND create_time >= $2)
		`
		var deltaStr string
		var deltaBal decimal.Decimal
		if err := session.QueryRowCtx(ctx, &deltaStr, deltaQuery, daily.UserId, deltaStartTime); err != nil {
			return err
		}
		if val, err := decimal.NewFromString(deltaStr); err == nil {
			deltaBal = val
		}

		// 3. 全局余额 = 快照 + 增量
		// 全局余额代表今日还未扣减消费金额时的可用余额
		globalBalance := snapshotBal.Add(deltaBal)

		// 查候选充值记录：FEFO 排序
		// 业务语义：只要在消费发生那一天内还未过期的批次，都可以用于结算这一天的消费
		// 因此按“消费日的下一天零点”作为有效期边界，而不是按当前 time.Now()
		// 只有在整天（2 月 4 日 00:00–24:00）都没有过期的充值批次，才允许用来结算 2 月 4 日的消费。
		// 批次A expire_time = 2025-02-05 00:30:00，用户在2月4日白天消费 100，定时核销任务是在2月5日 01:00 跑，在 2 月 4 日整天内，这个批次确实是“未过期”的；
		// 但在 2 月 5 日 01:00 结算那一刻，它已经过期半小时了。所以在 2 月 5 日 01:00 跑的时候，这个批次会被当成“已过期，不可用”，于是不能参与 2 月 4 日那笔消费的核销。
		// 在2月4日用户消费时 记录A确实是未过期的，我们核销时应该按照用户花费当天未过期为基准，而不是以当前核销时间过期为基准
		dayEnd := daily.Day.AddDate(0, 0, 1) // 消费日的次日 00:00:00
		query := `
			SELECT id, user_id, xlcredit_amount, pay_time, expire_time, related_recharge_id
			FROM ae_user_recharge_record
			WHERE user_id = $1 AND xlcredit_amount > 0 AND (expire_time IS NULL OR expire_time >= $2)
			ORDER BY expire_time ASC, pay_time ASC
		`
		var candidates []rechargeRecord //记录待扣减的充值记录
		if err := session.QueryRowsCtx(ctx, &candidates, query, daily.UserId, dayEnd); err != nil {
			return err
		}

		//【核心修复】如果没有有效记录，去捞取最近一条充值记录(不管过期，是否有钱，是否负数)，保证欠款有地方挂，而不是直接忽略
		if len(candidates) == 0 {
			fallbackQuery := `
				SELECT id, user_id, xlcredit_amount, pay_time, expire_time, related_recharge_id
				FROM ae_user_recharge_record
				WHERE user_id = $1
				ORDER BY pay_time DESC
				LIMIT 1
			`
			var fallbackRecord rechargeRecord
			err := session.QueryRowCtx(ctx, &fallbackRecord, fallbackQuery, daily.UserId)
			if err != nil {
				if err == sql.ErrNoRows {
					// 真的没有任何记录（用户从未充值过却产生了消费？），属于严重的业务异常
					j.Errorf("[SettlementJob] 严重异常：用户 %s 没有任何历史充值记录，无法挂载消费 %s", daily.UserId, consumeAmount.String())
					return nil
				}
				return err
			}
			j.Infof("[SettlementJob] 用户 %s 无有效充值记录，兜底使用历史记录(ID=%d)进行挂账", daily.UserId, fallbackRecord.Id)
			candidates = append(candidates, fallbackRecord)
		}

		//TODO【仅用某条充值记录计算透支量有局限性】
		//// A.代数和法
		//// 先扫一遍看看这个用户之前欠了多少钱
		//// 如果不加这一步，就会误以为新批次的钱全是能用的，忽略了旧批次的透支，所以我们应该先进行填补逻辑
		//accumulatedDebt := decimal.Zero //定义一个负帐账本
		//for _, rec := range candidates {
		//	initialAmount := decimal.NewFromFloat(rec.XlcreditAmount)
		//	// 利用 balance_helper 计算物理余额 (DB里真实剩多少)
		//	physBal, err := fund.CalculateRealTimeBalance(ctx, session, rec.Id, initialAmount)
		//	if err != nil {
		//		return err
		//	}
		//	//如果物理余额是负的(说明是挂载了超额消费的充值记录)，加入负帐账本
		//	if physBal.LessThan(decimal.Zero) {
		//		accumulatedDebt = accumulatedDebt.Add(physBal.Abs())
		//	}
		//}

		//B.执行扣款
		for i, rec := range candidates {
			//如果待核销金额是0，提前结束
			if amountToDeduct.LessThanOrEqual(decimal.Zero) {
				break
			}

			//1.获取物理余额
			initialAmount := decimal.NewFromFloat(rec.XlcreditAmount)
			balance, err := fund.CalculateRealTimeBalance(ctx, session, rec.Id, initialAmount)
			if err != nil {
				j.Errorf("[SettlementJob] 计算批次 %d 余额失败: %v", rec.Id, err)
				return err
				//TODO:所有计算失败之后应该做什么
			}

			// 判断是否是最后一条记录
			isLastRecord := (i == len(candidates)-1)

			//2.如果当前记录已经是负的(已经透支过),直接跳过
			//它没资格参与扣款，它的债已经记录在上面的 accumulatedDebt 里了
			//[修正跳过逻辑]只有当它不是最后一条记录时才跳过，如果是最后一条，即使已透支也要继续被消费记录映射
			// 场景解释：假设昨天A透支变为-50(当时是最后一条);今天充了 B，A 变成了倒数第二条。
			// 此时遍历到 A，因为它已不再是最后一条且已透支，必须跳过，不能再扣它，而是利用代数和法在后面扣 B。
			// 假设第三天B也被扣光了变为-10，如果不写 !isLastRecord 条件，A和B都会被跳过，实际应该用B继续去承担
			if !isLastRecord && balance.LessThanOrEqual(decimal.Zero) {
				continue
			}

			// 2. 计算“有效余额” (Effective Balance)
			// 核心逻辑：单条记录的购买力 = Min(物理余额, 全局剩余总资产)
			var effectiveBalance decimal.Decimal

			// --- 分支 A：全局都已经欠费了 ---
			if globalBalance.LessThanOrEqual(decimal.Zero) {
				// 如果 Global Balance <= 0 (比如是 -50)，说明用户不仅没钱，还欠着平台的钱。
				// 这种情况下，无论当前这条记录(Record B)本身显示还有多少钱，
				// 它在逻辑上已经被拿去填补之前的巨额窟窿了，一分钱都拿不出来。
				effectiveBalance = decimal.Zero
			} else {
				// --- 分支 B：全局还有钱 (我们的场景走这里) ---
				// Global Balance = 110 > 0

				// 比较：物理余额(200) vs 全局余额(110)
				if balance.LessThan(globalBalance) {
					// 子分支 B1: 物理余额 < 全局余额
					// 这种情况通常发生在：用户有很充裕的钱，没有历史欠账。
					// 比如 B 有 200，Global 有 300 (说明还有别的卡 C 也有钱)。
					// 既然 B 的 200 块是干净的，那就全都能用。
					effectiveBalance = balance
				} else {
					// 子分支 B2: 物理余额 >= 全局余额 (我们的场景走这里！)
					// 物理(200) >= 全局(110)

					// 这说明：虽然 B 账面上显示有 200，但其中有 90 块钱是“虚”的，
					// 已经被逻辑上拿去填补历史的烂账(A的-90)了。
					// 所以，B 真正能拿出来消费的“有效金额”，被“全局水位”限制住了。
					effectiveBalance = globalBalance // 取 110
				}
			}

			//3.决定本次扣多少
			var actualDeduct decimal.Decimal

			if isLastRecord {
				// 【死磕逻辑】
				// 如果是最后一条，不管有效余额够不够，必须扛下所有剩余消费！
				// 哪怕 effectiveBalance 是 0，这里也要扣 remainToDeduct，导致透支
				actualDeduct = amountToDeduct
			} else {
				// 【正常逻辑】
				// 它是中间的充值记录，且余额 > 0。只能扣它“有效”的部分，不能透支
				if effectiveBalance.GreaterThanOrEqual(amountToDeduct) {
					actualDeduct = amountToDeduct
				} else {
					actualDeduct = effectiveBalance
				}
			}

			//5.执行落库 (Insert Only)
			//只有当本次计算出的actualDeduct是一个正数时，才执行数据库插入操作，保证核销表金额有效
			if actualDeduct.GreaterThan(decimal.Zero) {
				now := time.Now()
				//保证幂等性，已经存在的消费id，充值记录id，只有可能插入一次
				insertQuery := `
					INSERT INTO ae_recharge_allocation (consumption_daily_id, recharge_record_id, deducted_amount, create_time, update_time)
					VALUES ($1, $2, $3, $4, $5)
					ON CONFLICT (consumption_daily_id, recharge_record_id) 
					DO NOTHING
				`
				result, err := session.ExecCtx(ctx, insertQuery, daily.Id, rec.Id, actualDeduct, now, now)
				if err != nil {
					return err
				}
				rowsAffected, _ := result.RowsAffected()
				if rowsAffected == 0 {
					// 已存在分摊记录（ON CONFLICT DO NOTHING），仍需扣减内存中的剩余待核销额，否则重试时会误报「仍有剩余」
					var existingDeductStr string
					if qErr := session.QueryRowCtx(ctx, &existingDeductStr,
						"SELECT COALESCE(deducted_amount::text, '0') FROM ae_recharge_allocation WHERE consumption_daily_id = $1 AND recharge_record_id = $2",
						daily.Id, rec.Id); qErr != nil {
						j.Errorf("[SettlementJob] 幂等分支查询已存在金额失败: daily=%d, recharge=%d, err=%v", daily.Id, rec.Id, qErr)
						return qErr
					}
					existingDeduct, _ := decimal.NewFromString(existingDeductStr)
					//【修复】这里不应该用actualDeduct，如果时重跑时保证幂等，用库中已存在的 deducted_amount 扣减，保证重复执行时 amountToDeduct 与 DB 一致
					//amountToDeduct = amountToDeduct.Sub(actualDeduct)
					amountToDeduct = amountToDeduct.Sub(existingDeduct)
					j.Info("[SettlementJob] 幂等跳过: 记录(daily=%d, recharge=%d)已存在，已扣减库中金额=%s，剩余待扣=%s", daily.Id, rec.Id, existingDeductStr, amountToDeduct.String())

					continue
				}
				newAllocCount++
				expireTimeStr := "永久有效"
				//数据库里面有值就用对应的真实时间，为null就用永久有效
				if rec.ExpireTime.Valid {
					expireTimeStr = rec.ExpireTime.Time.Format(time.RFC3339)
				}
				j.Infof("[SettlementJob] 核销详情: user_id=%s, 扣减金额=%s, 充值批次ID=%d, 批次过期时间=%s, 操作时间=%s, 剩余需扣=%s",
					daily.UserId, actualDeduct.String(), rec.Id, expireTimeStr, now.Format(time.RFC3339), amountToDeduct.Sub(actualDeduct).String())

				//更新剩余待扣余额
				amountToDeduct = amountToDeduct.Sub(actualDeduct)
			}
		}

		//6.最终检查
		//由于有 “无论最后一条有效充值记录够不够，都要承担剩余的所有待扣减”
		if amountToDeduct.GreaterThan(decimal.Zero) {
			j.Errorf("[SettlementJob] 用户 %s 日消费 %v 核销异常，逻辑执行完毕仍有剩余: %v", daily.UserId, daily.XlcreditConsume, amountToDeduct)
		}
		return nil
	})
	return newAllocCount, err
}
