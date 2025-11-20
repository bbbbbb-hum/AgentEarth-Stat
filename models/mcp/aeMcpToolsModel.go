package mcp

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeMcpToolsModel = (*customAeMcpToolsModel)(nil)

type (
	// AeMcpToolsModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeMcpToolsModel.
	AeMcpToolsModel interface {
		aeMcpToolsModel
		withSession(session sqlx.Session) AeMcpToolsModel
		FindByServiceId(ctx context.Context, serviceId int64) ([]*AeMcpTools, error)
	}

	customAeMcpToolsModel struct {
		*defaultAeMcpToolsModel
	}
)

// NewAeMcpToolsModel returns a model for the database table.
func NewAeMcpToolsModel(conn sqlx.SqlConn) AeMcpToolsModel {
	return &customAeMcpToolsModel{
		defaultAeMcpToolsModel: newAeMcpToolsModel(conn),
	}
}

func (m *customAeMcpToolsModel) withSession(session sqlx.Session) AeMcpToolsModel {
	return NewAeMcpToolsModel(sqlx.NewSqlConnFromSession(session))
}

// FindByServiceId 根据服务ID查询所有工具
func (m *customAeMcpToolsModel) FindByServiceId(ctx context.Context, serviceId int64) ([]*AeMcpTools, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE service_id = $1 ORDER BY name", aeMcpToolsRows, m.table)

	// 通过原生连接执行查询
	rawDB, err := m.conn.RawDB()
	if err != nil {
		return nil, err
	}
	rows, err := rawDB.QueryContext(ctx, query, serviceId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tools []*AeMcpTools
	for rows.Next() {
		var tool AeMcpTools
		err = rows.Scan(&tool.Id, &tool.ServiceId, &tool.Name, &tool.Description,
			&tool.ArgsSchema, &tool.CreateTime, &tool.UpdateTime)
		if err != nil {
			continue
		}
		tools = append(tools, &tool)
	}

	return tools, nil
}
