package fund

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeRechargeAllocationModel = (*customAeRechargeAllocationModel)(nil)

type (
	// AeRechargeAllocationModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeRechargeAllocationModel.
	AeRechargeAllocationModel interface {
		aeRechargeAllocationModel
		WithSession(session sqlx.Session) AeRechargeAllocationModel
		// 统计侧：按日消费汇总已核销、查询已存在金额、插入核销（ON CONFLICT DO NOTHING）
		GetAllocatedSumByConsumptionDailyId(ctx context.Context, consumptionDailyId int64) (string, error)
		GetExistingDeductedAmount(ctx context.Context, consumptionDailyId, rechargeRecordId int64) (string, error)
		InsertAllocation(ctx context.Context, consumptionDailyId, rechargeRecordId int64, deductedAmount decimal.Decimal, createTime, updateTime time.Time) (int64, error)
	}

	customAeRechargeAllocationModel struct {
		*defaultAeRechargeAllocationModel
	}
)

// NewAeRechargeAllocationModel returns a model for the database table.
func NewAeRechargeAllocationModel(conn sqlx.SqlConn) AeRechargeAllocationModel {
	return &customAeRechargeAllocationModel{
		defaultAeRechargeAllocationModel: newAeRechargeAllocationModel(conn),
	}
}

func (m *customAeRechargeAllocationModel) WithSession(session sqlx.Session) AeRechargeAllocationModel {
	return NewAeRechargeAllocationModel(sqlx.NewSqlConnFromSession(session))
}

// GetAllocatedSumByConsumptionDailyId 查询某条日消费记录已核销金额总和，用于幂等与剩余待扣计算。
// 表：ae_recharge_allocation。返回 COALESCE(SUM(deducted_amount), 0) 的字符串，便于上层用 decimal 解析。
func (m *customAeRechargeAllocationModel) GetAllocatedSumByConsumptionDailyId(ctx context.Context, consumptionDailyId int64) (string, error) {
	const query = `SELECT COALESCE(SUM(deducted_amount), 0) FROM ae_recharge_allocation WHERE consumption_daily_id = $1`
	var s string
	if err := m.conn.QueryRowCtx(ctx, &s, query, consumptionDailyId); err != nil {
		return "", err
	}
	return s, nil
}

// GetExistingDeductedAmount 查询已存在的核销记录金额，用于 ON CONFLICT DO NOTHING 后重试时从库中取已扣金额，保证 amountToDeduct 与 DB 一致。
func (m *customAeRechargeAllocationModel) GetExistingDeductedAmount(ctx context.Context, consumptionDailyId, rechargeRecordId int64) (string, error) {
	const query = `SELECT COALESCE(deducted_amount::text, '0') FROM ae_recharge_allocation WHERE consumption_daily_id = $1 AND recharge_record_id = $2`
	var s string
	if err := m.conn.QueryRowCtx(ctx, &s, query, consumptionDailyId, rechargeRecordId); err != nil {
		return "", err
	}
	return s, nil
}

// InsertAllocation 插入一条核销记录到 ae_recharge_allocation；若 (consumption_daily_id, recharge_record_id) 已存在则 ON CONFLICT DO NOTHING。
// 保证幂等：同一消费记录+充值记录只插入一次。返回 RowsAffected：0 表示已存在（幂等），1 表示新插入。
func (m *customAeRechargeAllocationModel) InsertAllocation(ctx context.Context, consumptionDailyId, rechargeRecordId int64, deductedAmount decimal.Decimal, createTime, updateTime time.Time) (int64, error) {
	const query = `
		INSERT INTO ae_recharge_allocation (consumption_daily_id, recharge_record_id, deducted_amount, create_time, update_time)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (consumption_daily_id, recharge_record_id)
		DO NOTHING
	`
	result, err := m.conn.ExecCtx(ctx, query, consumptionDailyId, rechargeRecordId, deductedAmount, createTime, updateTime)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

