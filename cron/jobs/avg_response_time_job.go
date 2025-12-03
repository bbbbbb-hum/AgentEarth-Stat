package jobs

import (
	"AgentEarth-Stat/cron/internal/svc"
	"AgentEarth-Stat/models"
	"AgentEarth-Stat/models/mcp"
	"context"
	"math"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// AvgResponseTimeJob 计算返回时间平均值
type AvgResponseTimeJob struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAvgResponseTimeJob(ctx context.Context, svcCtx *svc.ServiceContext) *AvgResponseTimeJob {
	return &AvgResponseTimeJob{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (j *AvgResponseTimeJob) RunOld() {
	j.Infof("AvgResponseTimeJob started at: %s", time.Now().Format("2006-01-02 15:04:05"))
	//获取所有服务列表
	services, servicesTotal, err := j.svcCtx.McpServiceModel.GetList(j.ctx, models.ListConditions{}, true)
	if err != nil {
		j.Error(err)
		return
	}
	var successTotal int
	if servicesTotal > 0 {
		for _, service := range services {
			//获取服务的请求列表
			responseTime, err1 := j.svcCtx.McpServiceRequestLogsModel.GetAvgResponseTime(j.ctx, service.ServerId)
			if err1 != nil {
				j.Errorf("GetAvgResponseTime error: %v", err1)
				continue
			}
			if responseTime > 0 {
				// 对齐到两位小数再比较
				newCenti := math.Round(responseTime * 100)
				oldCenti := math.Round(service.ResponseTime * 100)

				j.Debug("newCenti:", newCenti, "oldCenti:", oldCenti)
				if newCenti > 0 && oldCenti != newCenti {
					service.ResponseTime = newCenti / 100
					// 更新平均请求时间
					err1 = j.svcCtx.McpServiceModel.Update(j.ctx, service)
					if err1 != nil {
						j.Errorf("Update error: %v", err1)
						continue
					}
					successTotal++
				}
			}
		}
	}
	j.Infof("AvgResponseTimeJob completed,servicesTotal: %d,successTotal:%d", servicesTotal, successTotal)
}

func (j *AvgResponseTimeJob) Run() {
	// 获取上一个小时整点请求列表
	currentHourStart := time.Now().Truncate(time.Hour)
	prevHourStart := currentHourStart.Add(-1 * time.Hour)
	requestLogs, requestLogsTotal, err := j.svcCtx.McpServiceRequestLogsModel.GetList(j.ctx, models.ListConditions{
		Conditions: []models.Condition{
			{
				Field:  "request_time",
				Symbol: ">=",
				Value:  prevHourStart,
			},
			{
				Field:  "request_time",
				Symbol: "<",
				Value:  currentHourStart,
			},
		},
	}, true)
	// 循环请求列表，按服务维度统计上一个小时的请求次数和平均响应时间，并且按工具维度统计上一个小时的请求次数和平均响应时间
	if requestLogsTotal > 0 {
		// 按服务维度统计上一个小时的请求次数和平均响应时间
		serviceStatisticMap := make(map[string]mcp.AeMcpServicesStatistic)
		// 按工具维度统计上一个小时的请求次数和平均响应时间
		serviceToolsStatisticMap := make(map[string]mcp.AeMcpServicesStatisticTools)
		for _, requestLog := range requestLogs {
			// 从 serviceStatisticMap 中获取服务统计信息
			serviceStatistic, ok := serviceStatisticMap[requestLog.ServerId]
			if !ok {
				serviceStatistic = mcp.AeMcpServicesStatistic{
					ServerId:     requestLog.ServerId,
					Year:         int64(prevHourStart.Year()),
					Month:        int64(prevHourStart.Month()),
					Day:          int64(prevHourStart.Day()),
					Hour:         int64(prevHourStart.Hour()),
					RequestTotal: 0,
					ResponseTime: 0,
				}
			}

			// 从 serviceToolsStatisticMap 中获取工具统计信息
			serviceToolsStatistic, ok := serviceToolsStatisticMap[requestLog.ToolName]
			if !ok {
				serviceToolsStatistic = mcp.AeMcpServicesStatisticTools{
					ServerId:     requestLog.ServerId,
					ToolName:     requestLog.ToolName,
					Year:         int64(prevHourStart.Year()),
					Month:        int64(prevHourStart.Month()),
					Day:          int64(prevHourStart.Day()),
					Hour:         int64(prevHourStart.Hour()),
					RequestTotal: 0,
					ResponseTime: 0,
				}
			}
			// 累加请求次数和响应时间
			if requestLog.RequestTime.Unix() > 0 {
				serviceStatistic.RequestTotal++
				serviceStatistic.ResponseTime += float64(requestLog.ResponseTime) //先将响应时间累加，最后再计算平均值

				serviceToolsStatistic.RequestTotal++
				serviceToolsStatistic.ResponseTime += float64(requestLog.ResponseTime) //先将响应时间累加，最后再计算平均值
			}
			serviceStatisticMap[requestLog.ServerId] = serviceStatistic
			serviceToolsStatisticMap[requestLog.ToolName] = serviceToolsStatistic
		}
		// 计算服务的平均响应时间
		serviceStatisticList := make([]mcp.AeMcpServicesStatistic, 0)
		for _, serviceStatistic := range serviceStatisticMap {
			if serviceStatistic.RequestTotal > 0 && serviceStatistic.ResponseTime > 0 {
				serviceStatistic.ResponseTime = serviceStatistic.ResponseTime / float64(serviceStatistic.RequestTotal)
				serviceStatisticList = append(serviceStatisticList, serviceStatistic)
			}
		}
		// 批量插入服务统计信息
		serviceStatisticNum := len(serviceStatisticList)
		if serviceStatisticNum > 0 {
			err = j.svcCtx.McpServicesStatisticModel.BatchInsert(j.ctx, serviceStatisticList)
			if err != nil {
				j.Errorf("BatchInsert serviceStatistic error: %v", err)
			}
		}
		j.Infof("AvgResponseTimeJob First step completed,serviceStatisticTotal: %d", serviceStatisticNum)
		// 计算工具的平均响应时间
		serviceToolsStatisticList := make([]mcp.AeMcpServicesStatisticTools, 0)
		for _, serviceToolsStatistic := range serviceToolsStatisticMap {
			if serviceToolsStatistic.RequestTotal > 0 && serviceToolsStatistic.ResponseTime > 0 {
				serviceToolsStatistic.ResponseTime = serviceToolsStatistic.ResponseTime / float64(serviceToolsStatistic.RequestTotal)
				serviceToolsStatisticList = append(serviceToolsStatisticList, serviceToolsStatistic)
			}
		}
		// 批量插入工具统计信息
		serviceToolsStatisticNum := len(serviceToolsStatisticList)
		if serviceToolsStatisticNum > 0 {
			err = j.svcCtx.McpServicesStatisticToolsModel.BatchInsert(j.ctx, serviceToolsStatisticList)
			if err != nil {
				j.Errorf("BatchInsert serviceToolsStatistic error: %v", err)
			}
		}
		j.Infof("AvgResponseTimeJob Second step completed,serviceToolsStatisticTotal: %d", serviceToolsStatisticNum)
		//计算过去24小时内的服务的平均响应时间
		serviceStatisticList24, serviceTotal24, err24 := j.svcCtx.McpServicesStatisticModel.GetList(j.ctx, models.ListConditions{
			Conditions: []models.Condition{
				{
					Field:  "year",
					Symbol: "=",
					Value:  int64(currentHourStart.Year()),
				},
				{
					Field:  "month",
					Symbol: "=",
					Value:  int64(currentHourStart.Month()),
				},
				{
					Field:  "day",
					Symbol: "=",
					Value:  int64(currentHourStart.Day()),
				},
				{
					Field:  "hour",
					Symbol: ">=",
					Value:  int64(currentHourStart.Hour() - 24),
				},
				{
					Field:  "hour",
					Symbol: "<",
					Value:  int64(currentHourStart.Hour()),
				},
			},
		}, true)
		if err24 != nil {
			j.Errorf("GetList serviceStatistic error: %v", err24)
		}
		var mcpServicesResponseTimeTotalMap = make(map[string]McpServicesResponseTimeTotal)
		if len(serviceStatisticList24) > 0 {
			for _, serviceStatistic := range serviceStatisticList24 {
				if value, ok := mcpServicesResponseTimeTotalMap[serviceStatistic.ServerId]; ok {
					value.ResponseTimeTotal += serviceStatistic.ResponseTime
					value.Total++
					mcpServicesResponseTimeTotalMap[serviceStatistic.ServerId] = value
				} else {
					mcpServicesResponseTimeTotalMap[serviceStatistic.ServerId] = McpServicesResponseTimeTotal{
						ResponseTimeTotal: serviceStatistic.ResponseTime,
						Total:             1,
					}
				}

			}
		}
		// 获取服务列表
		services, servicesTotal, err := j.svcCtx.McpServiceModel.GetList(j.ctx, models.ListConditions{}, true)
		if err != nil {
			j.Errorf("GetList services error: %v", err)
		}
		var successTotal int
		if servicesTotal > 0 {
			for _, service := range services {
				if value, ok := mcpServicesResponseTimeTotalMap[service.ServerId]; ok {
					service.ResponseTime = value.ResponseTimeTotal / float64(value.Total)
					err = j.svcCtx.McpServiceModel.Update(j.ctx, service)
					if err != nil {
						j.Errorf("Update service error: %v", err)
					}
					successTotal++
				}
			}
		}
		j.Infof("AvgResponseTimeJob Third step completed,servicesTotal: %d,successTotal:%d", serviceTotal24, successTotal)
	}
}

type McpServicesResponseTimeTotal struct {
	ResponseTimeTotal float64 `json:"response_time_total"`
	Total             int64   `json:"request_total"`
}
