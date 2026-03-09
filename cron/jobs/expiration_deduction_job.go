package jobs

import (
	"AgentEarth-Stat/cron/internal/fund"
	"AgentEarth-Stat/cron/internal/svc"
	fundmodel "AgentEarth-Stat/models/fund"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
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

// 过期扣减算法逻辑流程图见：https://kcnh6cevaeaq.feishu.cn/wiki/CriGwqkejioBSNk7QRccoMjXnHd
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
		// 收集本页需要做过期扣减的记录，后面一次性批量插入；失败时降级为逐条插入
		type pendingItem struct {
			record fundmodel.ExpiredRechargeRecordRow
			params fundmodel.ExpirationDeductionParams
		}
		var pending []pendingItem

		for _, record := range expiredRecords {
			totalChecked++
			lastId = record.Id

			initialAmount := decimal.NewFromFloat(record.XlcreditAmount)
			balance, err := fund.CalculateRealTimeBalance(j.ctx, j.svcCtx.UserRechargeRecordModel, record.Id, initialAmount)
			if err != nil {
				j.Errorf("[ExpirationDeductionJob] 计算批次 %d 余额失败: %v", record.Id, err)
				continue
			}
			if balance.LessThanOrEqual(decimal.Zero) {
				continue
			}

			now := time.Now()
			remark := fmt.Sprintf("充值记录 %d 到期自动清理", record.Id)
			pending = append(pending, pendingItem{
				record: record,
				params: fundmodel.ExpirationDeductionParams{
					UserId:            record.UserId,
					NegativeAmount:    balance.Neg(),
					Now:               now,
					ChargeSource:      5,
					ChargeType:        141,
					Remark:            remark,
					RelatedRechargeID: record.Id,
					Operator:          "System_Auto",
				},
			})
		}

		if len(pending) == 0 {
			continue
		}

		// 批量插入本页需要扣减的记录（单条 INSERT 多行，本身原子；失败时重试 + 降级为逐条）
		params := make([]fundmodel.ExpirationDeductionParams, 0, len(pending))
		for _, item := range pending {
			params = append(params, item.params)
		}
		const maxBatchRetries = 2
		var batchErr error
		for attempt := 1; attempt <= maxBatchRetries; attempt++ {
			batchErr = j.svcCtx.UserRechargeRecordModel.BatchInsertExpirationDeductionRecords(j.ctx, params)
			if batchErr == nil {
				break
			}
			j.Errorf("[ExpirationDeductionJob] 批量插入过期扣减记录失败(第 %d 次): %v", attempt, batchErr)
		}

		if batchErr != nil {
			// 多次重试仍失败，降级为逐条插入，避免整页丢失
			j.Errorf("[ExpirationDeductionJob] 批量插入多次失败，开始逐条重试当前批次")
			for _, item := range pending {
				deducted, procErr := j.processSingleRecord(item.record)
				if procErr != nil {
					j.Errorf("[ExpirationDeductionJob] 逐条重试批次 %d 失败: %v", item.record.Id, procErr)
					continue
				}
				if deducted {
					totalDeducted++
				}
			}
			continue
		}

		// 批量成功，按条记日志，语义与原先逐条版本一致
		for _, item := range pending {
			totalDeducted++
			expireStr := "永久有效"
			if item.record.ExpireTime.Valid {
				expireStr = item.record.ExpireTime.Time.Format(time.RFC3339)
			}
			j.Infof("[ExpirationDeductionJob] 核销详情: 用户=%s, 过期扣减金额=%s, 充值批次ID=%d, 批次过期时间=%s, 操作时间=%s, 备注=%s",
				item.record.UserId, item.params.NegativeAmount.Abs().String(), item.record.Id, expireStr, item.params.Now.Format(time.RFC3339), item.params.Remark)
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
			if isUniqueViolation(err) {
				return nil
			}
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

// isUniqueViolation 判断是否为 PostgreSQL 唯一约束冲突（23505），用于过期扣减插入时幂等跳过。
func isUniqueViolation(err error) bool {
	var e *pq.Error
	if errors.As(err, &e) {
		return e.Code == "23505"
	}
	return false
}
