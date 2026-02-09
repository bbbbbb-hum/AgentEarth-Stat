package jobs

import (
	"AgentEarth-Stat/cron/internal/fund"
	"AgentEarth-Stat/cron/internal/svc"
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// ExpirationDeductionJob 过期扣减：扫描已过期的正向充值批次，计算剩余余额并插入负值"过期回收"记录到 ae_user_recharge_record
type ExpirationDeductionJob struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewExpirationDeductionJob(ctx context.Context, svcCtx *svc.ServiceContext) *ExpirationDeductionJob {
	return &ExpirationDeductionJob{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

type expiredRechargeRecord struct {
	Id             int64        `db:"id"`
	UserId         string       `db:"user_id"`
	XlcreditAmount float64      `db:"xlcredit_amount"`
	ExpireTime     sql.NullTime `db:"expire_time"`
}

func (j *ExpirationDeductionJob) Run() {
	j.Infof("[ExpirationDeductionJob] 开始扫描已过期充值记录")
	if err := j.processExpiration(); err != nil {
		j.Errorf("[ExpirationDeductionJob] 处理失败: %v", err)
	}
}

func (j *ExpirationDeductionJob) processExpiration() error {
	query := `
		SELECT id, user_id, xlcredit_amount, expire_time
		FROM ae_user_recharge_record
		WHERE expire_time < NOW() AND xlcredit_amount > 0
	`
	var expiredRecords []expiredRechargeRecord
	if err := j.svcCtx.DB.QueryRowsCtx(j.ctx, &expiredRecords, query); err != nil {
		j.Errorf("[ExpirationDeductionJob] 查询过期记录失败: %v", err)
		return err
	}
	if len(expiredRecords) == 0 {
		j.Infof("[ExpirationDeductionJob] 未发现需要处理的过期充值记录")
		return nil
	}
	j.Infof("[ExpirationDeductionJob] 共 %d 条过期记录待检查", len(expiredRecords))
	deductedCount := 0
	for _, record := range expiredRecords {
		deducted, procErr := j.processSingleRecord(record)
		if procErr != nil {
			j.Errorf("[ExpirationDeductionJob] 处理批次 %d 失败: %v", record.Id, procErr)
			continue
		}
		if deducted {
			deductedCount++
		}
	}
	j.Infof("[ExpirationDeductionJob] 完成: 检查=%d, 本次扣减=%d", len(expiredRecords), deductedCount)
	return nil
}

func (j *ExpirationDeductionJob) processSingleRecord(record expiredRechargeRecord) (bool, error) {
	var deducted bool
	err := j.svcCtx.DB.TransactCtx(j.ctx, func(ctx context.Context, session sqlx.Session) error {
		// 幂等检查：若已存在该批次的过期扣减记录，则跳过
		var existsCount int64
		if err := session.QueryRowCtx(ctx, &existsCount,
			"SELECT COUNT(*) FROM ae_user_recharge_record WHERE related_recharge_id = $1 AND xlcredit_amount < 0 AND charge_type = 4",
			record.Id); err != nil {
			return fmt.Errorf("幂等检查查询失败: %w", err)
		}
		if existsCount > 0 {
			return nil
		}
		initialAmount := decimal.NewFromFloat(record.XlcreditAmount)
		balance, err := fund.CalculateRealTimeBalance(ctx, session, record.Id, initialAmount)
		if err != nil {
			return err
		}
		if balance.LessThanOrEqual(decimal.Zero) {
			return nil
		}
		negativeAmount := balance.Neg()
		insertQuery := `
			INSERT INTO ae_user_recharge_record (
				user_id, xlcredit_amount, pay_time, create_time, update_time,
				charge_source, charge_type, remark, related_recharge_id, operator
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`
		now := time.Now()
		remark := fmt.Sprintf("充值记录 %d 到期自动清理", record.Id)
		chargeSource := int64(-2)
		chargeType := int64(4)
		_, err = session.ExecCtx(ctx, insertQuery,
			record.UserId, negativeAmount, now, now, now,
			chargeSource, chargeType, remark, record.Id, "System_Auto")
		if err != nil {
			return err
		}
		deducted = true
		expireStr := "永久有效"
		if record.ExpireTime.Valid {
			expireStr = record.ExpireTime.Time.Format(time.RFC3339)
		}
		j.Infof("[ExpirationDeductionJob] 核销详情: 用户=%s, 过期扣减金额=%s, 充值批次ID=%d, 批次过期时间=%s, 操作时间=%s, 备注=%s",
			record.UserId, balance.String(), record.Id, expireStr, now.Format(time.RFC3339), remark)
		return nil
	})
	return deducted, err
}
