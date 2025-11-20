package users

import (
	"context"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeUserTokensModel = (*customAeUserTokensModel)(nil)

type (
	// AeUserTokensModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeUserTokensModel.
	AeUserTokensModel interface {
		aeUserTokensModel
		withSession(session sqlx.Session) AeUserTokensModel
		FindOneByToken(ctx context.Context, token string) (*AeUserTokens, error)
	}

	customAeUserTokensModel struct {
		*defaultAeUserTokensModel
	}
)

// NewAeUserTokensModel returns a model for the database table.
func NewAeUserTokensModel(conn sqlx.SqlConn) AeUserTokensModel {
	return &customAeUserTokensModel{
		defaultAeUserTokensModel: newAeUserTokensModel(conn),
	}
}

func (m *customAeUserTokensModel) withSession(session sqlx.Session) AeUserTokensModel {
	return NewAeUserTokensModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customAeUserTokensModel) FindOneByToken(ctx context.Context, token string) (*AeUserTokens, error) {
	var resp AeUserTokens
	query := "select id, user_id, token, expires_at, create_time, update_time, status from public.ae_user_tokens where token = $1 limit 1"
	err := m.conn.QueryRowCtx(ctx, &resp, query, token)
	if err == sqlx.ErrNotFound {
		return nil, ErrNotFound
	}
	return &resp, err
}
