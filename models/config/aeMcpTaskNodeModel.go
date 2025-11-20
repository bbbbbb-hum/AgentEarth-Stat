package config

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeMcpTaskNodeModel = (*customAeMcpTaskNodeModel)(nil)

type (
	// AeMcpTaskNodeModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeMcpTaskNodeModel.
	AeMcpTaskNodeModel interface {
		aeMcpTaskNodeModel
		withSession(session sqlx.Session) AeMcpTaskNodeModel
		// InsertReturningId inserts and returns id (PostgreSQL RETURNING)
		InsertReturningId(ctx context.Context, data *AeMcpTaskNode) (int64, error)
	}

	customAeMcpTaskNodeModel struct {
		*defaultAeMcpTaskNodeModel
	}
)

// NewAeMcpTaskNodeModel returns a model for the database table.
func NewAeMcpTaskNodeModel(conn sqlx.SqlConn) AeMcpTaskNodeModel {
	return &customAeMcpTaskNodeModel{
		defaultAeMcpTaskNodeModel: newAeMcpTaskNodeModel(conn),
	}
}

func (m *customAeMcpTaskNodeModel) withSession(session sqlx.Session) AeMcpTaskNodeModel {
	return NewAeMcpTaskNodeModel(sqlx.NewSqlConnFromSession(session))
}

// InsertReturningId inserts a record and returns the generated id using RETURNING.
func (m *customAeMcpTaskNodeModel) InsertReturningId(ctx context.Context, data *AeMcpTaskNode) (int64, error) {
	query := fmt.Sprintf("insert into %s (%s) values ($1, $2, $3, $4, $5) returning id", m.table, aeMcpTaskNodeRowsExpectAutoSet)
	var id int64
	if err := m.conn.QueryRowCtx(ctx, &id, query, data.NodeName, data.NodeHandle, data.Enabled, data.ExternalServiceId, data.Description); err != nil {
		return 0, err
	}
	return id, nil
}
