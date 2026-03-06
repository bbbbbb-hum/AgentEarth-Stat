package svc

import (
	"AgentEarth-Stat/cron/internal/config"
	"AgentEarth-Stat/models/mcp"
	"AgentEarth-Stat/models/users"

	// 引入你需要的 model

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config                         config.Config
	DB                             sqlx.SqlConn
	Redis                          *redis.Redis
	NatsConn                       *nats.Conn
	JetStream                      jetstream.JetStream
	McpServiceModel                mcp.AeMcpServicesModel
	McpServiceRequestLogsModel     mcp.AeMcpServicesRequestLogsModel
	McpServicesStatisticModel      mcp.AeMcpServicesStatisticModel
	McpServicesStatisticToolsModel mcp.AeMcpServicesStatisticToolsModel
	UserBalanceStatisticDailyModel users.AeUserBalanceStatisticDailyModel
}

func NewServiceContext(c config.Config) *ServiceContext {
	db := sqlx.NewSqlConn("postgres", c.DB.DataSource)

	// 初始化Redis
	rds, err := redis.NewRedis(c.Redis)
	if err != nil {
		logx.Errorf("Redis connected field: %v", err)
	} else {
		logx.Info("Redis connected successfully")
	}

	return &ServiceContext{
		Config:                         c,
		DB:                             db,
		Redis:                          rds,
		McpServiceModel:                mcp.NewAeMcpServicesModel(db),
		McpServiceRequestLogsModel:     mcp.NewAeMcpServicesRequestLogsModel(db),
		McpServicesStatisticModel:      mcp.NewAeMcpServicesStatisticModel(db),
		McpServicesStatisticToolsModel: mcp.NewAeMcpServicesStatisticToolsModel(db),
		UserBalanceStatisticDailyModel: users.NewAeUserBalanceStatisticDailyModel(db),
	}
}

func (sc *ServiceContext) IsReady() bool {

	// 检查 Redis 连接是否正常
	if sc.Redis == nil {
		return false
	}
	res := sc.Redis.Ping()
	if res != true {
		return res
	}

	// 检查数据库连接是否正常（如果有数据库）
	db, err := sc.DB.RawDB()
	if err != nil {
		return false
	}
	if db == nil || db.Ping() != nil {
		return false
	}
	return true
}
