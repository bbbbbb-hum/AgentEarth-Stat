package fund

import (
	"context"
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
		// 统计侧：按日查询有消费的记录，用于日结核销遍历
		QueryDailyConsumptionByDay(ctx context.Context, dateStr string) ([]DailyRecordRow, error)
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

// QueryDailyConsumptionByDay 查询指定日期下有消费的日消费记录，用于日结核销 job。
// 表：ae_user_consumption_record_daily。只返回 xlcredit_consume > 0 的记录。
func (m *customAeUserConsumptionRecordDailyModel) QueryDailyConsumptionByDay(ctx context.Context, dateStr string) ([]DailyRecordRow, error) {
	const query = `
		SELECT id, user_id, day, xlcredit_consume, create_time
		FROM ae_user_consumption_record_daily
		WHERE day = $1 AND xlcredit_consume > 0
	`
	var list []DailyRecordRow
	if err := m.conn.QueryRowsCtx(ctx, &list, query, dateStr); err != nil {
		return nil, err
	}
	return list, nil
}

