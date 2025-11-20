package users

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var _ AeUserModel = (*customAeUserModel)(nil)

type (
	// AeUserModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeUserModel.
	AeUserModel interface {
		aeUserModel
		withSession(session sqlx.Session) AeUserModel
	}

	customAeUserModel struct {
		*defaultAeUserModel
	}
)

// NewAeUserModel returns a model for the database table.
func NewAeUserModel(conn sqlx.SqlConn) AeUserModel {
	return &customAeUserModel{
		defaultAeUserModel: newAeUserModel(conn),
	}
}

func (m *customAeUserModel) withSession(session sqlx.Session) AeUserModel {
	return NewAeUserModel(sqlx.NewSqlConnFromSession(session))
}
