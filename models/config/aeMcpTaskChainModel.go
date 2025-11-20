package config

import (
	"AgentEarth-Stat/models"
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeMcpTaskChainModel = (*customAeMcpTaskChainModel)(nil)

type (
	// AeMcpTaskChainModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeMcpTaskChainModel.
	AeMcpTaskChainModel interface {
		aeMcpTaskChainModel
		withSession(session sqlx.Session) AeMcpTaskChainModel
		// InsertReturningId inserts and returns id (PostgreSQL RETURNING)
		InsertReturningId(ctx context.Context, data *AeMcpTaskChain) (int64, error)
		GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeMcpTaskChain, total int64, err error)
	}

	customAeMcpTaskChainModel struct {
		*defaultAeMcpTaskChainModel
	}
)

// NewAeMcpTaskChainModel returns a model for the database table.
func NewAeMcpTaskChainModel(conn sqlx.SqlConn) AeMcpTaskChainModel {
	return &customAeMcpTaskChainModel{
		defaultAeMcpTaskChainModel: newAeMcpTaskChainModel(conn),
	}
}

func (m *customAeMcpTaskChainModel) withSession(session sqlx.Session) AeMcpTaskChainModel {
	return NewAeMcpTaskChainModel(sqlx.NewSqlConnFromSession(session))
}

// InsertReturningId inserts a record and returns the generated id using RETURNING.
func (m *customAeMcpTaskChainModel) InsertReturningId(ctx context.Context, data *AeMcpTaskChain) (int64, error) {
	query := fmt.Sprintf("insert into %s (%s) values ($1, $2, $3) returning id", m.table, aeMcpTaskChainRowsExpectAutoSet)
	var id int64
	if err := m.conn.QueryRowCtx(ctx, &id, query, data.Name, data.Status, data.NodeIds); err != nil {
		return 0, err
	}
	return id, nil
}

func (m *customAeMcpTaskChainModel) GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeMcpTaskChain, total int64, err error) {
	countQuery := fmt.Sprintf("select count(*) as number from %s", m.table)
	query := fmt.Sprintf("select %s from %s", aeMcpTaskChainRows, m.table)
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
