package mcp

import (
	"AgentEarth-Stat/models"
	"context"
	"fmt"

	"database/sql"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeMcpServicesModel = (*customAeMcpServicesModel)(nil)

type (
	// AeMcpServicesModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeMcpServicesModel.
	AeMcpServicesModel interface {
		aeMcpServicesModel
		withSession(session sqlx.Session) AeMcpServicesModel
		GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeMcpServices, total int64, err error)
		GetAllTags(ctx context.Context) []string
		// InsertWithoutId inserts a service row without setting the auto-increment id column
		InsertWithoutId(ctx context.Context, data *AeMcpServices) (sql.Result, error)
	}

	customAeMcpServicesModel struct {
		*defaultAeMcpServicesModel
	}
)

// NewAeMcpServicesModel returns a model for the database table.
func NewAeMcpServicesModel(conn sqlx.SqlConn) AeMcpServicesModel {
	return &customAeMcpServicesModel{
		defaultAeMcpServicesModel: newAeMcpServicesModel(conn),
	}
}

func (m *customAeMcpServicesModel) withSession(session sqlx.Session) AeMcpServicesModel {
	return NewAeMcpServicesModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customAeMcpServicesModel) GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeMcpServices, total int64, err error) {
	countQuery := fmt.Sprintf("select count(*) as number from %s", m.table)
	query := fmt.Sprintf("select %s from %s", aeMcpServicesRows, m.table)
	lp.Conditions = append(lp.Conditions, models.Condition{
		Field: "enabled",
		Value: true,
	})
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

func (m *customAeMcpServicesModel) GetAllTags(ctx context.Context) []string {
	var tags []string
	query := fmt.Sprintf("SELECT DISTINCT unnest(tags) as tag FROM %s WHERE tags IS NOT NULL AND array_length(tags, 1) > 0 ORDER BY tag", m.table)
	err := m.conn.QueryRowsCtx(ctx, &tags, query)
	if err != nil {
		return []string{}
	}
	return tags
}

// InsertWithoutId inserts without providing the auto-increment id.
func (m *customAeMcpServicesModel) InsertWithoutId(ctx context.Context, data *AeMcpServices) (sql.Result, error) {
	// Explicit column list without id
	columns := "server_id, server_name, logo, protocol_version, enabled, tags, description, task_chain_id, x_net_service_id, call_num,project_name"
	query := fmt.Sprintf("insert into %s (%s) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)", m.table, columns)
	return m.conn.ExecCtx(ctx, query, data.ServerId, data.ServerName, data.Logo, data.ProtocolVersion, data.Enabled, data.Tags, data.Description, data.TaskChainId, data.XNetServiceId, data.CallNum, data.ProjectName)
}
