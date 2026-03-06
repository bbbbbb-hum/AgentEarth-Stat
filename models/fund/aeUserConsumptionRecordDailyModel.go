package fund

import (
	"AgentEarth-Stat/models/mcp"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ AeUserConsumptionRecordDailyModel = (*customAeUserConsumptionRecordDailyModel)(nil)

type (
	// AeUserConsumptionRecordDailyModel is an interface to be customized, add more methods here,
	// and implement the added methods in customAeUserConsumptionRecordDailyModel.
	AeUserConsumptionRecordDailyModel interface {
		aeUserConsumptionRecordDailyModel
		WithSession(session sqlx.Session) AeUserConsumptionRecordDailyModel
		// 统计侧：按日查询有消费的记录，用于日结核销遍历。
		// 使用基于 id 的游标分页查询指定日期的日消费记录，按 id 递增，每次最多返回 limit 条。
		// 用于大数据量场景，避免一次性加载过多记录到内存。
		QueryDailyConsumptionByDay(ctx context.Context, dateStr string, lastId, limit int64) ([]DailyRecordRow, error)
		// UpsertDailyConsume 批量写入/更新某一天的用户日消费统计（来自调用日志的聚合结果），幂等可重跑。
		UpsertDailyConsume(ctx context.Context, day time.Time, items []mcp.UserDailyConsume) error
	}

	customAeUserConsumptionRecordDailyModel struct {
		*defaultAeUserConsumptionRecordDailyModel
	}
)

// DailyRecordRow 日消费记录，对应 ae_user_consumption_record_daily 一行，用于日结核销遍历。
type DailyRecordRow struct {
	Id              int64     `db:"id"`
	UserId          string    `db:"user_id"`
	Day             time.Time `db:"day"`
	XlcreditConsume float64   `db:"xlcredit_consume"`
	CreateTime      time.Time `db:"create_time"`
}

// NewAeUserConsumptionRecordDailyModel returns a model for the database table.
func NewAeUserConsumptionRecordDailyModel(conn sqlx.SqlConn) AeUserConsumptionRecordDailyModel {
	return &customAeUserConsumptionRecordDailyModel{
		defaultAeUserConsumptionRecordDailyModel: newAeUserConsumptionRecordDailyModel(conn),
	}
}

func (m *customAeUserConsumptionRecordDailyModel) WithSession(session sqlx.Session) AeUserConsumptionRecordDailyModel {
	return NewAeUserConsumptionRecordDailyModel(sqlx.NewSqlConnFromSession(session))
}

// QueryDailyConsumptionByDay 基于 id 的游标分页查询指定日期的日消费记录，用于日结核销 job。
// 表：ae_user_consumption_record_daily。只返回 xlcredit_consume > 0 的记录，按 id 递增，每次最多返回 limit 条。
func (m *customAeUserConsumptionRecordDailyModel) QueryDailyConsumptionByDay(ctx context.Context, dateStr string, lastId, limit int64) ([]DailyRecordRow, error) {
	const query = `
		SELECT id, user_id, day, xlcredit_consume, create_time
		FROM ae_user_consumption_record_daily
		WHERE day = $1 AND xlcredit_consume > 0 AND id > $2
		ORDER BY id ASC
		LIMIT $3
	`
	var list []DailyRecordRow
	if err := m.conn.QueryRowsCtx(ctx, &list, query, dateStr, lastId, limit); err != nil {
		return nil, err
	}
	return list, nil
}

// UpsertDailyConsume 批量写入/更新某一天的用户日消费统计（来自调用日志的聚合结果），幂等可重跑。
// 依赖 (day, user_id) 上的唯一约束，使用 ON CONFLICT 覆盖 xlcredit_consume，保证多次运行结果一致。
func (m *customAeUserConsumptionRecordDailyModel) UpsertDailyConsume(ctx context.Context, day time.Time, items []mcp.UserDailyConsume) error {
	if len(items) == 0 {
		return nil
	}

	// 只关心日期部分，时间由数据库 date 类型截断
	dayDate := day.Format("2006-01-02")

	// 每条记录 3 个参数：user_id, day, xlcredit_consume
	const colsPerRow = 3
	values := make([]string, 0, len(items))
	args := make([]interface{}, 0, len(items)*colsPerRow)

	for i, it := range items {
		// ($1,$2,$3), ($4,$5,$6) ...
		base := i*colsPerRow + 1
		values = append(values, fmt.Sprintf("($%d, $%d, $%d)", base, base+1, base+2))
		args = append(args,
			it.UserId,
			dayDate,
			it.Amount, // numeric(20,8) 可直接用字符串承接，避免浮点误差
		)
	}

	query := fmt.Sprintf(`
		INSERT INTO ae_user_consumption_record_daily (user_id, day, xlcredit_consume)
		VALUES %s
		ON CONFLICT (day, user_id)
		DO UPDATE SET xlcredit_consume = EXCLUDED.xlcredit_consume
	`, strings.Join(values, ","))

	_, err := m.conn.ExecCtx(ctx, query, args...)
	return err
}

