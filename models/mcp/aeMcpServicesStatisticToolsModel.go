package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeMcpServicesStatisticToolsModel = (*customAeMcpServicesStatisticToolsModel)(nil)

type (
	// AeMcpServicesStatisticToolsModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeMcpServicesStatisticToolsModel.
	AeMcpServicesStatisticToolsModel interface {
		aeMcpServicesStatisticToolsModel
		withSession(session sqlx.Session) AeMcpServicesStatisticToolsModel
		BatchInsert(ctx context.Context, list []AeMcpServicesStatisticTools) error
	}

	customAeMcpServicesStatisticToolsModel struct {
		*defaultAeMcpServicesStatisticToolsModel
	}
)

// NewAeMcpServicesStatisticToolsModel returns a model for the database table.
func NewAeMcpServicesStatisticToolsModel(conn sqlx.SqlConn) AeMcpServicesStatisticToolsModel {
	return &customAeMcpServicesStatisticToolsModel{
		defaultAeMcpServicesStatisticToolsModel: newAeMcpServicesStatisticToolsModel(conn),
	}
}

func (m *customAeMcpServicesStatisticToolsModel) withSession(session sqlx.Session) AeMcpServicesStatisticToolsModel {
	return NewAeMcpServicesStatisticToolsModel(sqlx.NewSqlConnFromSession(session))
}

// BatchInsert inserts multiple rows for performance.
func (m *customAeMcpServicesStatisticToolsModel) BatchInsert(ctx context.Context, list []AeMcpServicesStatisticTools) error {
	if len(list) == 0 {
		return nil
	}
	columns := []string{
		"server_id",
		"tool_name",
		"year",
		"month",
		"day",
		"hour",
		"response_time",
		"request_total",
	}
	var (
		sb   strings.Builder
		args []interface{}
		idx  = 1
	)
	sb.WriteString("insert into ")
	sb.WriteString(m.defaultAeMcpServicesStatisticToolsModel.table)
	sb.WriteString(" (")
	sb.WriteString(strings.Join(columns, ","))
	sb.WriteString(") values ")
	for i, v := range list {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(")
		for j := 0; j < len(columns); j++ {
			if j > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf("$%d", idx))
			idx++
		}
		sb.WriteString(")")
		args = append(args,
			v.ServerId,
			v.ToolName,
			v.Year,
			v.Month,
			v.Day,
			v.Hour,
			v.ResponseTime,
			v.RequestTotal,
		)
	}
	_, err := m.defaultAeMcpServicesStatisticToolsModel.conn.ExecCtx(ctx, sb.String(), args...)
	return err
}
