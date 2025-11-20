package svc

import (
	"AgentEarth-Stat/cron/internal/config"
	"AgentEarth-Stat/models/mcp"

	// 引入你需要的 model

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config                         config.Config
	DB                             sqlx.SqlConn
	McpServiceModel                mcp.AeMcpServicesModel
	McpServiceRequestLogsModel     mcp.AeMcpServicesRequestLogsModel
	McpServicesStatisticModel      mcp.AeMcpServicesStatisticModel
	McpServicesStatisticToolsModel mcp.AeMcpServicesStatisticToolsModel
}

func NewServiceContext(c config.Config) *ServiceContext {
	db := sqlx.NewSqlConn("postgres", c.DB.DataSource)
	return &ServiceContext{
		Config:                         c,
		DB:                             db,
		McpServiceModel:                mcp.NewAeMcpServicesModel(db),
		McpServiceRequestLogsModel:     mcp.NewAeMcpServicesRequestLogsModel(db),
		McpServicesStatisticModel:      mcp.NewAeMcpServicesStatisticModel(db),
		McpServicesStatisticToolsModel: mcp.NewAeMcpServicesStatisticToolsModel(db),
	}
}
