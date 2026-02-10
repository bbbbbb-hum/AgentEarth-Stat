package fund

import (
	"context"

	"github.com/shopspring/decimal"
)

// FundModelForBalance 仅需实时余额计算，避免 cron/internal/fund 与 models/fund 重名时循环依赖。
type FundModelForBalance interface {
	CalculateRealTimeBalance(ctx context.Context, recordID int64, initialAmount decimal.Decimal) (decimal.Decimal, error)
}

// CalculateRealTimeBalance 计算特定充值记录的实时余额，委托给 model 层实现。
// 实现逻辑：当前余额 = initialAmount - 消费扣减总和(ae_recharge_allocation) - 系统扣减总和(ae_user_recharge_record 负值)。
// 调用方传入 svcCtx.FundModel 或 svcCtx.FundModel.WithSession(session) 以支持事务内调用。
func CalculateRealTimeBalance(ctx context.Context, model FundModelForBalance, recordID int64, initialAmount decimal.Decimal) (decimal.Decimal, error) {
	return model.CalculateRealTimeBalance(ctx, recordID, initialAmount)
}
