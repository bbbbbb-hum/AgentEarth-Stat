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
	var totalNewAllocs int64
	for _, record := range dailyRecords {
		n, procErr := j.processUserDailyConsumption(&record)
		if procErr != nil {
			j.Errorf("[SettlementJob] 用户 %s 核销失败: %v", record.UserId, procErr)
			continue
		}
		totalNewAllocs += n
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
	// 幂等检查：已完全分摊则跳过
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
		// 事务内按“已分摊金额”重算剩余待扣，避免部分成功后重跑从全额开始扣导致过度分摊
		var allocatedSumStr string
		if err := session.QueryRowCtx(ctx, &allocatedSumStr,
			"SELECT COALESCE(SUM(deducted_amount), 0) FROM ae_recharge_allocation WHERE consumption_daily_id = $1", daily.Id); err != nil {
			// 查询已分摊金额失败时，直接中断事务，避免退化为“从消费总额重新扣”导致过度核销
			return err
		}
		if allocatedSum, parseErr := decimal.NewFromString(allocatedSumStr); parseErr == nil {
			amountToDeduct = consumeAmount.Sub(allocatedSum)
		}
		if amountToDeduct.LessThanOrEqual(decimal.Zero) {
			return nil
		}

		// 查候选充值记录：FEFO 排序
		// 业务语义：只要在消费发生那一天内还未过期的批次，都可以用于结算这一天的消费
		// 因此按“消费日的下一天零点”作为有效期边界，而不是按当前 time.Now()
		//只有在整天（2 月 4 日 00:00–24:00）都没有过期的充值批次，才允许用来结算 2 月 4 日的消费。
		dayEnd := daily.Day.AddDate(0, 0, 1) // 消费日的次日 00:00:00
		query := `
			SELECT id, user_id, xlcredit_amount, pay_time, expire_time, related_recharge_id
			FROM ae_user_recharge_record
			WHERE user_id = $1 AND xlcredit_amount > 0 AND (expire_time IS NULL OR expire_time >= $2)
			ORDER BY expire_time ASC, pay_time ASC
		`
		var candidates []rechargeRecord
		if err := session.QueryRowsCtx(ctx, &candidates, query, daily.UserId, dayEnd); err != nil {
			return err
		}
		for _, rec := range candidates {
			if amountToDeduct.LessThanOrEqual(decimal.Zero) {
				break
			}
			initialAmount := decimal.NewFromFloat(rec.XlcreditAmount)
			balance, err := fund.CalculateRealTimeBalance(ctx, session, rec.Id, initialAmount)
			if err != nil {
				j.Errorf("[SettlementJob] 计算批次 %d 余额失败: %v", rec.Id, err)
				return err
			}
			if balance.LessThanOrEqual(decimal.Zero) {
				continue
			}
			var actualDeduct decimal.Decimal
			if balance.GreaterThanOrEqual(amountToDeduct) {
				actualDeduct = amountToDeduct
			} else {
				actualDeduct = balance
			}
			now := time.Now()
			insertQuery := `
				INSERT INTO ae_recharge_allocation (consumption_daily_id, recharge_record_id, deducted_amount, create_time, update_time)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (consumption_daily_id, recharge_record_id) DO NOTHING
			`
			result, err := session.ExecCtx(ctx, insertQuery, daily.Id, rec.Id, actualDeduct, now, now)
			if err != nil {
				return err
			}
			rowsAffected, _ := result.RowsAffected()
			if rowsAffected == 0 {
				// 已存在分摊记录（ON CONFLICT DO NOTHING），不应使用本次计算的 actualDeduct 扣减剩余金额
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
			amountToDeduct = amountToDeduct.Sub(actualDeduct)
		}
		if amountToDeduct.GreaterThan(decimal.Zero) {
			j.Errorf("[SettlementJob] 用户 %s 日消费 %v 余额不足，剩余待扣: %v 需要人工补偿", daily.UserId, daily.XlcreditConsume, amountToDeduct)
		}
		return nil
	})
	return newAllocCount, err
}
