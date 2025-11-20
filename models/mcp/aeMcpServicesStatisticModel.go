package mcp

import (
	"AgentEarth-Stat/models"
	"context"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeMcpServicesStatisticModel = (*customAeMcpServicesStatisticModel)(nil)

type (
	// AeMcpServicesStatisticModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeMcpServicesStatisticModel.
	AeMcpServicesStatisticModel interface {
		aeMcpServicesStatisticModel
		withSession(session sqlx.Session) AeMcpServicesStatisticModel
		BatchInsert(ctx context.Context, list []AeMcpServicesStatistic) error
		GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeMcpServicesStatistic, total int64, err error)
	}

	customAeMcpServicesStatisticModel struct {
		*defaultAeMcpServicesStatisticModel
	}
)

// NewAeMcpServicesStatisticModel returns a model for the database table.
func NewAeMcpServicesStatisticModel(conn sqlx.SqlConn) AeMcpServicesStatisticModel {
	return &customAeMcpServicesStatisticModel{
		defaultAeMcpServicesStatisticModel: newAeMcpServicesStatisticModel(conn),
	}
}

func (m *customAeMcpServicesStatisticModel) withSession(session sqlx.Session) AeMcpServicesStatisticModel {
	return NewAeMcpServicesStatisticModel(sqlx.NewSqlConnFromSession(session))
}

// BatchInsert inserts multiple rows for performance.
func (m *customAeMcpServicesStatisticModel) BatchInsert(ctx context.Context, list []AeMcpServicesStatistic) error {
	if len(list) == 0 {
		return nil
	}
	columns := []string{
		"server_id",
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
	sb.WriteString(m.defaultAeMcpServicesStatisticModel.table)
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
			v.Year,
			v.Month,
			v.Day,
			v.Hour,
			v.ResponseTime,
			v.RequestTotal,
		)
	}
	_, err := m.defaultAeMcpServicesStatisticModel.conn.ExecCtx(ctx, sb.String(), args...)
	return err
}

// GetList returns paginated statistics with total count.
func (m *customAeMcpServicesStatisticModel) GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeMcpServicesStatistic, total int64, err error) {
	countQuery := fmt.Sprintf("select count(*) as number from %s", m.table)
	query := fmt.Sprintf("select %s from %s", aeMcpServicesStatisticRows, m.table)
	// where
	whereClause, args, err := models.DealWithWhereSafe(lp.Conditions...)
	if err != nil {
		return
	}
	countQuery += whereClause
	query += whereClause
	// count
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
		// order by
		if len(lp.Sorts) > 0 && len(lp.Sorts[0].Filed) > 0 && len(lp.Sorts[0].Order) > 0 {
			query = models.GetOrderBy(lp.Sorts, query)
		} else {
			query += " order by id desc"
		}
		// pagination
		if lp.Page > 0 && lp.Size > 0 {
			var offset = (lp.Page - 1) * lp.Size
			query += fmt.Sprintf(" limit %d offset %d", lp.Size, offset)
		}
		err = m.conn.QueryRowsCtx(ctx, &list, query, args...)
	}
	return
}
