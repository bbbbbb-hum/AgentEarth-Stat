package jobs

import (
	"AgentEarth-Stat/cron/internal/fund"
	"AgentEarth-Stat/cron/internal/svc"
	fundmodel "AgentEarth-Stat/models/fund"
	"context"
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

func (j *ExpirationDeductionJob) Run() {
	j.Infof("[ExpirationDeductionJob] 开始扫描已过期充值记录")
	if err := j.processExpiration(); err != nil {
		j.Errorf("[ExpirationDeductionJob] 处理失败: %v", err)
	}
}

func (j *ExpirationDeductionJob) processExpiration() error {
	const pageSize int64 = 1000

	var (
		lastId        int64 = 0
		totalChecked        = 0
		totalDeducted       = 0
	)

	for {
		expiredRecords, err := j.svcCtx.UserRechargeRecordModel.QueryExpiredRechargeRecords(j.ctx, lastId, pageSize)
		if err != nil {
			j.Errorf("[ExpirationDeductionJob] 查询过期记录失败: %v", err)
			return err
		}
		if len(expiredRecords) == 0 {
			break
		}

		j.Infof("[ExpirationDeductionJob] 本批次待检查过期记录数=%d, lastId=%d", len(expiredRecords), lastId)

		for _, record := range expiredRecords {
			totalChecked++
			lastId = record.Id

			deducted, procErr := j.processSingleRecord(record)
			if procErr != nil {
				j.Errorf("[ExpirationDeductionJob] 处理批次 %d 失败: %v", record.Id, procErr)
				continue
			}
			if deducted {
				totalDeducted++
			}
		}
	}

	if totalChecked == 0 {
		j.Infof("[ExpirationDeductionJob] 未发现需要处理的过期充值记录")
		return nil
	}

	j.Infof("[ExpirationDeductionJob] 完成: 总检查=%d, 本次总扣减=%d", totalChecked, totalDeducted)
	return nil
}

func (j *ExpirationDeductionJob) processSingleRecord(record fundmodel.ExpiredRechargeRecordRow) (bool, error) {
	var deducted bool
	err := j.svcCtx.DB.TransactCtx(j.ctx, func(ctx context.Context, session sqlx.Session) error {
		txRechargeModel := j.svcCtx.UserRechargeRecordModel.WithSession(session)

		// 幂等检查：若已存在该批次的过期扣减记录，则跳过
		existsCount, err := txRechargeModel.CountExpirationDeductionExists(ctx, record.Id)
		if err != nil {
			return fmt.Errorf("幂等检查查询失败: %w", err)
		}
		if existsCount > 0 {
			return nil
		}
		initialAmount := decimal.NewFromFloat(record.XlcreditAmount)
		balance, err := fund.CalculateRealTimeBalance(ctx, txRechargeModel, record.Id, initialAmount)
		if err != nil {
			return err
		}
		// 仅当该批次当前仍有正余额时才做“过期回收”：插入一条负值过期扣减记录。
		// balance <= 0 表示该批次已被完全用尽或已透支（例如 2.9 已把 100 消费挂到 50 的批次上），
		// 到期时视为“剩余 0”，不再插入过期扣减记录，避免重复记账；透支金额由 OverdraftAmount 展示为 50。
		if balance.LessThanOrEqual(decimal.Zero) {
			return nil
		}
		negativeAmount := balance.Neg()
		now := time.Now()
		remark := fmt.Sprintf("充值记录 %d 到期自动清理", record.Id)
		err = txRechargeModel.InsertExpirationDeductionRecord(ctx, fundmodel.ExpirationDeductionParams{
			UserId:            record.UserId,
			NegativeAmount:    negativeAmount,
			Now:               now,
			ChargeSource:      5,
			ChargeType:        141,
			Remark:            remark,
			RelatedRechargeID: record.Id,
			Operator:          "System_Auto",
		})
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
