package users

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeUserBalanceStatisticDailyModel = (*customAeUserBalanceStatisticDailyModel)(nil)

type (
	// AeUserBalanceStatisticDailyModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeUserBalanceStatisticDailyModel.
	AeUserBalanceStatisticDailyModel interface {
		aeUserBalanceStatisticDailyModel
		withSession(session sqlx.Session) AeUserBalanceStatisticDailyModel
		FindOneLastDayByUserId(ctx context.Context, userId string) (*AeUserBalanceStatisticDaily, error)
	}

	customAeUserBalanceStatisticDailyModel struct {
		*defaultAeUserBalanceStatisticDailyModel
	}
)

// NewAeUserBalanceStatisticDailyModel returns a model for the database table.
func NewAeUserBalanceStatisticDailyModel(conn sqlx.SqlConn) AeUserBalanceStatisticDailyModel {
	return &customAeUserBalanceStatisticDailyModel{
		defaultAeUserBalanceStatisticDailyModel: newAeUserBalanceStatisticDailyModel(conn),
	}
}

func (m *customAeUserBalanceStatisticDailyModel) withSession(session sqlx.Session) AeUserBalanceStatisticDailyModel {
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
