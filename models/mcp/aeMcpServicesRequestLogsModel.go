package mcp

import (
	"AgentEarth-Stat/models"
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeMcpServicesRequestLogsModel = (*customAeMcpServicesRequestLogsModel)(nil)

type (
	// AeMcpServicesRequestLogsModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeMcpServicesRequestLogsModel.
	AeMcpServicesRequestLogsModel interface {
		aeMcpServicesRequestLogsModel
		withSession(session sqlx.Session) AeMcpServicesRequestLogsModel
		GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeMcpServicesRequestLogs, total int64, err error)
		GetAvgResponseTime(ctx context.Context, serverId string) (responseTime float64, err error)
	}

	customAeMcpServicesRequestLogsModel struct {
		*defaultAeMcpServicesRequestLogsModel
	}
)

// NewAeMcpServicesRequestLogsModel returns a model for the database table.
func NewAeMcpServicesRequestLogsModel(conn sqlx.SqlConn) AeMcpServicesRequestLogsModel {
	return &customAeMcpServicesRequestLogsModel{
		defaultAeMcpServicesRequestLogsModel: newAeMcpServicesRequestLogsModel(conn),
	}
}

func (m *customAeMcpServicesRequestLogsModel) withSession(session sqlx.Session) AeMcpServicesRequestLogsModel {
	return NewAeMcpServicesRequestLogsModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customAeMcpServicesRequestLogsModel) GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeMcpServicesRequestLogs, total int64, err error) {
	countQuery := fmt.Sprintf("select count(*) as number from %s", m.table)
	query := fmt.Sprintf("select %s from %s", aeMcpServicesRequestLogsRows, m.table)
	//lp.Conditions = append(lp.Conditions, models.Condition{
	//	Field: "enabled",
	//	Value: true,
	//})
	//处理where条件
	whereClause, args, err := models.DealWithWhereSafe(lp.Conditions...)
	if err != nil {
		return
	}
	countQuery += whereClause
	query += whereClause
	var totals []models.Total
	err = m.conn.QueryRowsCtx(ctx, &totals, countQuery, args...)
	if err != nil {
		return
	}
	total = totals[0].Number
	if total == 0 {
		return
	}
	if getList {
		//排序
		if len(lp.Sorts) > 0 && len(lp.Sorts[0].Filed) > 0 && len(lp.Sorts[0].Order) > 0 {
			query = models.GetOrderBy(lp.Sorts, query)
		} else {
			query += " order by id desc"
		}
		if lp.Page > 0 && lp.Size > 0 {
			var offset = (lp.Page - 1) * lp.Size
			query += fmt.Sprintf(" limit %d offset %d", lp.Size, offset)
		}
		err = m.conn.QueryRowsCtx(ctx, &list, query, args...)
	}
	return
}

func (m *customAeMcpServicesRequestLogsModel) GetAvgResponseTime(ctx context.Context, serverId string) (responseTime float64, err error) {
	query := fmt.Sprintf("select COALESCE(round(avg(response_time)::numeric, 2), 0)::double precision as response_time from %s where server_id = $1", m.table)
	err = m.conn.QueryRowCtx(ctx, &responseTime, query, serverId)
	if err != nil {
		return
	}
	return responseTime, nil
}
