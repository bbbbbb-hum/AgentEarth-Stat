package external

import (
	"AgentEarth-Stat/models"
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ ExternalMcpServicesModel = (*customExternalMcpServicesModel)(nil)

type (
	// ExternalMcpServicesModel is an interface to be customized, add more methods here,
	// and implement the added methods in customExternalMcpServicesModel.
	ExternalMcpServicesModel interface {
		externalMcpServicesModel
		withSession(session sqlx.Session) ExternalMcpServicesModel
		GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*ExternalMcpServices, total int64, err error)
		BatchUpdateTestStatus(ctx context.Context, ids []int64, status int64) error
	}

	customExternalMcpServicesModel struct {
		*defaultExternalMcpServicesModel
	}
)

// NewExternalMcpServicesModel returns a model for the database table.
func NewExternalMcpServicesModel(conn sqlx.SqlConn) ExternalMcpServicesModel {
	return &customExternalMcpServicesModel{
		defaultExternalMcpServicesModel: newExternalMcpServicesModel(conn),
	}
}

func (m *customExternalMcpServicesModel) withSession(session sqlx.Session) ExternalMcpServicesModel {
	return NewExternalMcpServicesModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customExternalMcpServicesModel) GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*ExternalMcpServices, total int64, err error) {
	countQuery := fmt.Sprintf("select count(*) as number from %s", m.table)
	query := fmt.Sprintf("select %s from %s", externalMcpServicesRows, m.table)
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

func (m *customExternalMcpServicesModel) BatchUpdateTestStatus(ctx context.Context, ids []int64, status int64) error {
	inCondition, inArgs := models.GetInCondition(ids)
	query := fmt.Sprintf("update %s set test_status = ? where %s", m.table, inCondition)
	args := append([]interface{}{status}, inArgs...)
	_, err := m.conn.ExecCtx(ctx, query, args...)
	if err != nil {
		return err
	}
	return nil
}
