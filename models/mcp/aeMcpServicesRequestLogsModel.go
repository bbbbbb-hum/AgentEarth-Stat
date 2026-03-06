package mcp

import (
	"AgentEarth-Stat/models"
	"context"
	"fmt"
	"strings"
	"time"

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
		InsertBatch(ctx context.Context, list []*AeMcpServicesRequestLogs) error
		GetAvgResponseTime(ctx context.Context, serverId string) (responseTime float64, err error)
		// AggregateUserDailyConsume 按 user_id 维度聚合某一天的总消费金额（xlcredit_amount），用于生成日消费统计。
		// day 表示统计的自然日，统计窗口为 [day 00:00:00, day+1 00:00:00)，仅统计 status=1 且 xlcredit_amount>0 的成功消费。
		AggregateUserDailyConsume(ctx context.Context, day time.Time) ([]UserDailyConsume, error)
	}

	customAeMcpServicesRequestLogsModel struct {
		*defaultAeMcpServicesRequestLogsModel
	}

	// UserDailyConsume 表示某个用户在某一天的总消费金额（从调用日志聚合而来）。
	UserDailyConsume struct {
		UserId string `db:"user_id"`
		Amount string `db:"total_consume"` // numeric 聚合结果以字符串形式承接，方便上层用 decimal 计算
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

func (m *customAeMcpServicesRequestLogsModel) InsertBatch(ctx context.Context, list []*AeMcpServicesRequestLogs) error {
	// 参数校验
	if len(list) == 0 {
		return nil
	}

	// 构建插入语句
	table := m.table
	columns := "server_id, tool_name, request_time, return_time, response_time, status, user_id, key_id, xlcredit_amount"
	values := make([]string, 0, len(list))
	args := make([]interface{}, 0, len(list)*9) // 每条记录有9个字段

	for i, item := range list {
		// 占位符索引从1开始
		placeholder := fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			i*9+1, i*9+2, i*9+3, i*9+4, i*9+5, i*9+6, i*9+7, i*9+8, i*9+9)
		values = append(values, placeholder)
		args = append(args,
			item.ServerId,
			item.ToolName,
			item.RequestTime,
			item.ReturnTime,
			item.ResponseTime,
			item.Status,
			item.UserId,
			item.KeyId,
			item.XlcreditAmount)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", table, columns, strings.Join(values, ","))

	// 执行批量插入
	_, err := m.conn.ExecCtx(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to insert batch: %w", err)
	}

	return nil
}

// AggregateUserDailyConsume 按 user_id 聚合某一天的总消费金额，用于生成日消费统计。
// 统计窗口为 [day 00:00:00, day+1 00:00:00)，仅统计 status=1 且 xlcredit_amount>0 的成功消费。
func (m *customAeMcpServicesRequestLogsModel) AggregateUserDailyConsume(ctx context.Context, day time.Time) ([]UserDailyConsume, error) {
	// 归一化到当天 00:00:00，保持与数据库时区一致（这里假定 day 已是正确时区）
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.AddDate(0, 0, 1)

	const query = `
		SELECT
			user_id,
			COALESCE(SUM(xlcredit_amount)::text, '0') AS total_consume
		FROM ae_mcp_services_request_logs
		WHERE
			request_time >= $1
			AND request_time < $2
			AND status = 1
			AND xlcredit_amount > 0
		GROUP BY user_id
	`

	var list []UserDailyConsume
	if err := m.conn.QueryRowsCtx(ctx, &list, query, start, end); err != nil {
		return nil, err
	}
	return list, nil
}
