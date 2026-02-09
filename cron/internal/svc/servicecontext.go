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

	// 初始化NATS连接
	nc := initNatsConn(c.Nats)

	// 初始化JetStream
	var js jetstream.JetStream
	if nc != nil {
		js, err = jetstream.New(nc)
		if err != nil {
			logx.Errorf("Failed to create JetStream context: %v", err)
		} else {
			logx.Info("JetStream context created successfully")
		}
	}

	return &ServiceContext{
		Config:                         c,
		DB:                             db,
		Redis:                          rds,
		NatsConn:                       nc,
		JetStream:                      js,
		McpServiceModel:                mcp.NewAeMcpServicesModel(db),
		McpServiceRequestLogsModel:     mcp.NewAeMcpServicesRequestLogsModel(db),
		McpServicesStatisticModel:      mcp.NewAeMcpServicesStatisticModel(db),
		McpServicesStatisticToolsModel: mcp.NewAeMcpServicesStatisticToolsModel(db),
		UserBalanceStatisticDailyModel: users.NewAeUserBalanceStatisticDailyModel(db),
	}
}

// initNatsConn 初始化NATS连接
func initNatsConn(cfg config.NatsConfig) *nats.Conn {
	if cfg.Url == "" {
		logx.Info("NATS URL is empty, skipping NATS connection")
		return nil
	}

	opts := []nats.Option{
		nats.Name("AgentEarth-Stat-Cron"),
		nats.ReconnectWait(nats.DefaultReconnectWait),
		nats.MaxReconnects(-1), // 无限重连
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logx.Errorf("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logx.Infof("NATS reconnected to %s", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logx.Info("NATS connection closed")
		}),
	}

	// 添加认证选项
	if cfg.Token != "" {
		opts = append(opts, nats.Token(cfg.Token))
	} else if cfg.Username != "" && cfg.Password != "" {
		opts = append(opts, nats.UserInfo(cfg.Username, cfg.Password))
	}

	nc, err := nats.Connect(cfg.Url, opts...)
	if err != nil {
		logx.Errorf("Failed to connect to NATS: %v", err)
		return nil
	}

	logx.Infof("Connected to NATS server: %s", nc.ConnectedUrl())
	return nc
}

// Close 关闭所有连接
func (s *ServiceContext) Close() {
	if s.NatsConn != nil {
		s.NatsConn.Drain()
		logx.Info("NATS connection drained and closed")
	}
}

func (sc *ServiceContext) IsReady() bool {
	// 检查 NATS 连接是否正常
	if sc.NatsConn == nil || !sc.NatsConn.IsConnected() {
		return false
	}

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
