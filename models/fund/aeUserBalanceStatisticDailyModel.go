package fund

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeUserBalanceStatisticDailyModel = (*customAeUserBalanceStatisticDailyModel)(nil)

type (
	// AeUserBalanceStatisticDailyModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeUserBalanceStatisticDailyModel.
	AeUserBalanceStatisticDailyModel interface {
		aeUserBalanceStatisticDailyModel
		WithSession(session sqlx.Session) AeUserBalanceStatisticDailyModel
		FindOneLastDayByUserId(ctx context.Context, userId string) (*AeUserBalanceStatisticDaily, error)
		// 统计侧：查询最新余额快照，用于日结核销 Snapshot+Delta
		QueryLatestBalanceSnapshot(ctx context.Context, userId string) (*BalanceSnapshotRow, error)
	}

	customAeUserBalanceStatisticDailyModel struct {
		*defaultAeUserBalanceStatisticDailyModel
	}
)

// BalanceSnapshotRow 日余额统计快照一行（ae_user_balance_statistic_daily），Balance 为 numeric 字符串，Day 为统计日。
type BalanceSnapshotRow struct {
	Balance string    `db:"balance"`
	Day     time.Time `db:"day"`
}

// NewAeUserBalanceStatisticDailyModel returns a model for the database table.
func NewAeUserBalanceStatisticDailyModel(conn sqlx.SqlConn) AeUserBalanceStatisticDailyModel {
	return &customAeUserBalanceStatisticDailyModel{
		defaultAeUserBalanceStatisticDailyModel: newAeUserBalanceStatisticDailyModel(conn),
	}
}

func (m *customAeUserBalanceStatisticDailyModel) WithSession(session sqlx.Session) AeUserBalanceStatisticDailyModel {
	return NewAeUserBalanceStatisticDailyModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customAeUserBalanceStatisticDailyModel) FindOneLastDayByUserId(ctx context.Context, userId string) (*AeUserBalanceStatisticDaily, error) {
	var resp AeUserBalanceStatisticDaily
	query := fmt.Sprintf("select %s from %s where user_id = $1 order by day desc limit 1", aeUserBalanceStatisticDailyRows, m.table)
	err := m.conn.QueryRowCtx(ctx, &resp, query, userId)
	switch err {
	case nil:
		return &resp, nil
	case sqlx.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// QueryLatestBalanceSnapshot 查询用户在日余额统计表中的最新快照。
// 表：ae_user_balance_statistic_daily，按 day DESC 取一条。无快照时视为新用户/从零开始算增量。
// 返回值：(*BalanceSnapshotRow, nil) 存在记录；(nil, nil) 无快照；(nil, err) 查询错误。
func (m *customAeUserBalanceStatisticDailyModel) QueryLatestBalanceSnapshot(ctx context.Context, userId string) (*BalanceSnapshotRow, error) {
	const query = `
		SELECT balance, day
		FROM ae_user_balance_statistic_daily
		WHERE user_id = $1
		ORDER BY day DESC
		LIMIT 1
	`
	var snap BalanceSnapshotRow
	if err := m.conn.QueryRowCtx(ctx, &snap, query, userId); err != nil {
		if errors.Is(err, sql.ErrNoRows) || err == sqlx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &snap, nil
}

