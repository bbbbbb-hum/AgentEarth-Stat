package users

import (
	"AgentEarth-Stat/models"
	"context"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeUserKeysModel = (*customAeUserKeysModel)(nil)

type (
	// AeUserKeysModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeUserKeysModel.
	AeUserKeysModel interface {
		aeUserKeysModel
		withSession(session sqlx.Session) AeUserKeysModel
		FindByUserId(ctx context.Context, userId string, page, size int64, search string) ([]*AeUserKeys, int64, error)
		FindByUserIdWithSearch(ctx context.Context, userId string, search string) ([]*AeUserKeys, error)
		GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeUserKeys, total int64, err error)
	}

	customAeUserKeysModel struct {
		*defaultAeUserKeysModel
	}
)

// NewAeUserKeysModel returns a model for the database table.
func NewAeUserKeysModel(conn sqlx.SqlConn) AeUserKeysModel {
	return &customAeUserKeysModel{
		defaultAeUserKeysModel: newAeUserKeysModel(conn),
	}
}

func (m *customAeUserKeysModel) withSession(session sqlx.Session) AeUserKeysModel {
	return NewAeUserKeysModel(sqlx.NewSqlConnFromSession(session))
}

// FindByUserId 根据用户ID分页查询密钥列表
func (m *customAeUserKeysModel) FindByUserId(ctx context.Context, userId string, page, size int64, search string) ([]*AeUserKeys, int64, error) {
	// 设置默认分页参数
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	if size > 100 {
		size = 100
	}

	// 构建查询条件
	whereClause := "user_id = $1"
	args := []interface{}{userId}
	argIndex := 2

	// 添加搜索条件
	if search != "" {
		whereClause += fmt.Sprintf(" AND (key_name ILIKE $%d OR key_value ILIKE $%d)", argIndex, argIndex)
		searchPattern := "%" + strings.ToLower(search) + "%"
		args = append(args, searchPattern)
		argIndex++
	}

	// 计算偏移量
	offset := (page - 1) * size

	// 查询总数
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", m.table, whereClause)
	var total int64
	err := m.conn.QueryRowCtx(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	// 查询密钥列表 - 使用原生连接执行多行查询
	listQuery := fmt.Sprintf(`
		SELECT %s FROM %s 
		WHERE %s 
		ORDER BY create_time DESC 
		LIMIT $%d OFFSET $%d
	`, aeUserKeysRows, m.table, whereClause, argIndex, argIndex+1)

	args = append(args, size, offset)

	// 通过原生连接执行查询
	rawDB, err := m.conn.RawDB()
	if err != nil {
		return nil, 0, err
	}
	rows, err := rawDB.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	// 解析结果
	var keys []*AeUserKeys
	for rows.Next() {
		var key AeUserKeys
		err = rows.Scan(&key.Id, &key.UserId, &key.KeyName, &key.KeyValue, &key.KeyType,
			&key.Status, &key.Permissions, &key.ExpiresAt, &key.LastUsedAt, &key.UsageCount,
			&key.CreateTime, &key.UpdateTime)
		if err != nil {
			continue
		}
		keys = append(keys, &key)
	}

	return keys, total, nil
}

// FindByUserIdWithSearch 根据用户ID和搜索条件查询密钥列表（不分页）
func (m *customAeUserKeysModel) FindByUserIdWithSearch(ctx context.Context, userId string, search string) ([]*AeUserKeys, error) {
	whereClause := "user_id = $1"
	args := []interface{}{userId}

	if search != "" {
		whereClause += " AND (key_name ILIKE $2 OR key_value ILIKE $2)"
		searchPattern := "%" + strings.ToLower(search) + "%"
		args = append(args, searchPattern)
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY created_at DESC", aeUserKeysRows, m.table, whereClause)

	// 通过原生连接执行查询
	rawDB, err := m.conn.RawDB()
	if err != nil {
		return nil, err
	}
	rows, err := rawDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*AeUserKeys
	for rows.Next() {
		var key AeUserKeys
		err = rows.Scan(&key.Id, &key.UserId, &key.KeyName, &key.KeyValue, &key.KeyType,
			&key.Status, &key.Permissions, &key.ExpiresAt, &key.LastUsedAt, &key.UsageCount,
			&key.CreateTime, &key.UpdateTime)
		if err != nil {
			continue
		}
		keys = append(keys, &key)
	}

	return keys, nil
}

func (m *customAeUserKeysModel) GetList(ctx context.Context, lp models.ListConditions, getList bool) (list []*AeUserKeys, total int64, err error) {
	countQuery := fmt.Sprintf("select count(*) as number from %s", m.table)
	query := fmt.Sprintf("select %s from %s", aeUserKeysRows, m.table)
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
