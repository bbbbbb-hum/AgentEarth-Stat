package fund

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeUserRechargeRecordModel = (*customAeUserRechargeRecordModel)(nil)

	type (
		// AeUserRechargeRecordModel is an interface to be customized, add more methods here,
		// and implement the added methods in customAeUserRechargeRecordModel.
		AeUserRechargeRecordModel interface {
			aeUserRechargeRecordModel
			WithSession(session sqlx.Session) AeUserRechargeRecordModel
			// 统计侧：资金增量、FEFO 候选、兜底、过期充值、幂等检查、过期扣减插入、实时余额
			QueryBalanceDeltaSince(ctx context.Context, userId string, from time.Time) (string, error)
			QueryRechargeCandidatesForSettlement(ctx context.Context, userId string, dayEnd time.Time) ([]RechargeRecordRow, error)
			QueryFallbackRechargeRecord(ctx context.Context, userId string) (*RechargeRecordRow, error)
			// QueryExpiredRechargeRecords 分页查询已过期且金额为正、且未做过过期扣减的充值记录，按 id 递增做 keyset 分页。
			// 参数 lastId 为上一页最后一条记录的 id（第一页传 0），limit 为每页条数。
			QueryExpiredRechargeRecords(ctx context.Context, lastId, limit int64) ([]ExpiredRechargeRecordRow, error)
			CountExpirationDeductionExists(ctx context.Context, rechargeId int64) (int64, error)
			InsertExpirationDeductionRecord(ctx context.Context, p ExpirationDeductionParams) error
			CalculateRealTimeBalance(ctx context.Context, recordID int64, initialAmount decimal.Decimal) (decimal.Decimal, error)
		}

	customAeUserRechargeRecordModel struct {
		*defaultAeUserRechargeRecordModel
	}
)

// RechargeRecordRow 充值记录，用于 FEFO 核销候选或兜底单条；含 related_recharge_id（过期扣减等关联用）。
type RechargeRecordRow struct {
	Id                int64         `db:"id"`
	UserId            string        `db:"user_id"`
	XlcreditAmount    float64       `db:"xlcredit_amount"`
	PayTime           time.Time     `db:"pay_time"`
	ExpireTime        sql.NullTime  `db:"expire_time"`
	RelatedRechargeId sql.NullInt64 `db:"related_recharge_id"`
}

// ExpiredRechargeRecordRow 已过期的正向充值记录（expire_time < NOW() 且 xlcredit_amount > 0），用于过期扣减 job 扫描。
type ExpiredRechargeRecordRow struct {
	Id             int64        `db:"id"`
	UserId         string       `db:"user_id"`
	XlcreditAmount float64      `db:"xlcredit_amount"`
	ExpireTime     sql.NullTime `db:"expire_time"`
}

// ExpirationDeductionParams 插入过期扣减记录所需参数；ChargeType=141 表示过期扣减，ChargeSource=5（非 cron_rule 业务逻辑，如过期扣减）、Operator 如 "System_Auto"。
type ExpirationDeductionParams struct {
	UserId            string
	NegativeAmount    decimal.Decimal
	Now               time.Time
	ChargeSource      int64
	ChargeType        int64
	Remark            string
	RelatedRechargeID int64
	Operator          string
}

// NewAeUserRechargeRecordModel returns a model for the database table.
func NewAeUserRechargeRecordModel(conn sqlx.SqlConn) AeUserRechargeRecordModel {
	return &customAeUserRechargeRecordModel{
		defaultAeUserRechargeRecordModel: newAeUserRechargeRecordModel(conn),
	}
}

func (m *customAeUserRechargeRecordModel) WithSession(session sqlx.Session) AeUserRechargeRecordModel {
	return NewAeUserRechargeRecordModel(sqlx.NewSqlConnFromSession(session))
}

// QueryBalanceDeltaSince 计算从指定起始时间（含）起的用户资金净增量。
// 公式：正向充值总额 - allocation 已核销金额 - 负向充值记录绝对值之和。
// 说明：重试场景下会把“上次失败遗留的扣款”纳入总账。返回 numeric 字符串供 decimal 解析。
func (m *customAeUserRechargeRecordModel) QueryBalanceDeltaSince(ctx context.Context, userId string, from time.Time) (string, error) {
	const query = `
		SELECT
			(SELECT COALESCE(SUM(xlcredit_amount), 0) FROM ae_user_recharge_record WHERE user_id = $1 AND xlcredit_amount > 0 AND create_time >= $2)
			- (SELECT COALESCE(SUM(deducted_amount), 0) FROM ae_recharge_allocation alloc JOIN ae_user_recharge_record rec ON alloc.recharge_record_id = rec.id WHERE rec.user_id = $1 AND alloc.create_time >= $2)
			- (SELECT COALESCE(SUM(ABS(xlcredit_amount)), 0) FROM ae_user_recharge_record WHERE user_id = $1 AND xlcredit_amount < 0 AND create_time >= $2)
	`
	var s string
	if err := m.conn.QueryRowCtx(ctx, &s, query, userId, from); err != nil {
		return "", err
	}
	return s, nil
}

// QueryRechargeCandidatesForSettlement 按 FEFO 查询可用于结算某日消费的充值候选。
// 表：ae_user_recharge_record。dayEnd 为消费日的次日 00:00:00，只有 expire_time IS NULL 或 expire_time >= dayEnd 的批次参与；
// 排序：expire_time ASC, pay_time ASC，先到期先扣。
func (m *customAeUserRechargeRecordModel) QueryRechargeCandidatesForSettlement(ctx context.Context, userId string, dayEnd time.Time) ([]RechargeRecordRow, error) {
	const query = `
		SELECT id, user_id, xlcredit_amount, pay_time, expire_time, related_recharge_id
		FROM ae_user_recharge_record
		WHERE user_id = $1 AND xlcredit_amount > 0 AND (expire_time IS NULL OR expire_time >= $2)
		ORDER BY expire_time ASC, pay_time ASC
	`
	var list []RechargeRecordRow
	if err := m.conn.QueryRowsCtx(ctx, &list, query, userId, dayEnd); err != nil {
		return nil, err
	}
	return list, nil
}

// QueryFallbackRechargeRecord 兜底查询用户最近一条“正向充值”记录，用于无有效候选时挂账，保证欠款有地方挂。
// 优先选择 xlcredit_amount > 0 的记录（无论是否过期），按 pay_time DESC, id DESC 取一条；
// 如用户从未有正向充值记录，仅存在负值扣减时，才退而求其次按“任意金额”取最近一条，避免消费记录完全找不到挂载点。
// 无任何记录时返回 (nil, nil)。
func (m *customAeUserRechargeRecordModel) QueryFallbackRechargeRecord(ctx context.Context, userId string) (*RechargeRecordRow, error) {
	// 1) 优先查最近一条正向充值（xlcredit_amount > 0）
	const queryPositive = `
		SELECT id, user_id, xlcredit_amount, pay_time, expire_time, related_recharge_id
		FROM ae_user_recharge_record
		WHERE user_id = $1 AND xlcredit_amount > 0
		ORDER BY pay_time DESC, id DESC
		LIMIT 1
	`
	var rec RechargeRecordRow
	err := m.conn.QueryRowCtx(ctx, &rec, queryPositive, userId)
	if err == nil {
		return &rec, nil
	}
	if !errors.Is(err, sql.ErrNoRows) && err != sqlx.ErrNotFound {
		// 其他错误直接返回
		return nil, err
	}

	// 2) 用户从未有正向充值，仅存在负值扣减等记录时，退而求其次按“任意金额”取最近一条，避免消费无处挂账
	const queryAny = `
		SELECT id, user_id, xlcredit_amount, pay_time, expire_time, related_recharge_id
		FROM ae_user_recharge_record
		WHERE user_id = $1
		ORDER BY pay_time DESC, id DESC
		LIMIT 1
	`
	if err := m.conn.QueryRowCtx(ctx, &rec, queryAny, userId); err != nil {
		if errors.Is(err, sql.ErrNoRows) || err == sqlx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
}

// QueryExpiredRechargeRecords 分页查询已过期且金额为正、且尚未做过过期扣减的充值记录，供过期扣减 job 扫描。
// 条件：expire_time < NOW() AND xlcredit_amount > 0 AND NOT EXISTS(对应的过期扣减记录) AND id > lastId。
// 使用按 id 递增的 keyset 分页，避免大 offset 带来的性能问题。
func (m *customAeUserRechargeRecordModel) QueryExpiredRechargeRecords(ctx context.Context, lastId, limit int64) ([]ExpiredRechargeRecordRow, error) {
	const query = `
		SELECT
			r.id,
			r.user_id,
			r.xlcredit_amount,
			r.expire_time
		FROM ae_user_recharge_record r
		WHERE
			r.expire_time < NOW()
			AND r.xlcredit_amount > 0
			AND r.id > $1
			AND NOT EXISTS (
				SELECT 1
				FROM ae_user_recharge_record d
				WHERE
					d.related_recharge_id = r.id
					AND d.xlcredit_amount < 0
					AND d.charge_type = 141
			)
		ORDER BY r.id ASC
		LIMIT $2
	`
	var list []ExpiredRechargeRecordRow
	if err := m.conn.QueryRowsCtx(ctx, &list, query, lastId, limit); err != nil {
		return nil, err
	}
	return list, nil
}

// CountExpirationDeductionExists 幂等检查：该充值批次是否已有过期扣减记录（ae_user_recharge_record 中 related_recharge_id 指向该批次且 xlcredit_amount < 0 且 charge_type = 141）。
func (m *customAeUserRechargeRecordModel) CountExpirationDeductionExists(ctx context.Context, rechargeId int64) (int64, error) {
	const query = `SELECT COUNT(*) FROM ae_user_recharge_record WHERE related_recharge_id = $1 AND xlcredit_amount < 0 AND charge_type = 141`
	var n int64
	if err := m.conn.QueryRowCtx(ctx, &n, query, rechargeId); err != nil {
		return 0, err
	}
	return n, nil
}

// InsertExpirationDeductionRecord 插入一条过期扣减记录到 ae_user_recharge_record（xlcredit_amount 为负值，charge_type=141，remark 如“充值记录 N 到期自动清理”）。
func (m *customAeUserRechargeRecordModel) InsertExpirationDeductionRecord(ctx context.Context, p ExpirationDeductionParams) error {
	const query = `
		INSERT INTO ae_user_recharge_record (
			user_id, xlcredit_amount, pay_time, create_time, update_time,
			charge_source, charge_type, remark, related_recharge_id, operator
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := m.conn.ExecCtx(ctx, query,
		p.UserId, p.NegativeAmount, p.Now, p.Now, p.Now,
		p.ChargeSource, p.ChargeType, p.Remark, p.RelatedRechargeID, p.Operator,
	)
	return err
}

// CalculateRealTimeBalance 计算指定充值记录的实时余额。
// 实现逻辑：当前余额 = initialAmount - 消费扣减总和(ae_recharge_allocation) - 系统/过期扣减总和(ae_user_recharge_record 负值且 related_recharge_id 指向该批次)。
func (m *customAeUserRechargeRecordModel) CalculateRealTimeBalance(ctx context.Context, recordID int64, initialAmount decimal.Decimal) (decimal.Decimal, error) {
	const query = `
		SELECT
			$2::numeric
			- COALESCE((SELECT SUM(deducted_amount) FROM ae_recharge_allocation WHERE recharge_record_id = $1), 0)
			- COALESCE((SELECT SUM(ABS(xlcredit_amount)) FROM ae_user_recharge_record WHERE related_recharge_id = $1 AND xlcredit_amount < 0), 0)
	`
	var currentBalanceStr string
	if err := m.conn.QueryRowCtx(ctx, &currentBalanceStr, query, recordID, initialAmount.String()); err != nil {
		return decimal.Zero, fmt.Errorf("计算充值记录 %d 的实时余额失败: %w", recordID, err)
	}
	currentBalance, err := decimal.NewFromString(currentBalanceStr)
	if err != nil {
		return decimal.Zero, fmt.Errorf("解析充值记录 %d 的余额结果失败: %w", recordID, err)
	}
	return currentBalance, nil
}

