package consumers

import (
	"AgentEarth-Stat/cron/internal/config"
	"AgentEarth-Stat/cron/internal/redis"
	"AgentEarth-Stat/cron/internal/svc"
	fundmodel "AgentEarth-Stat/models/fund"
	"AgentEarth-Stat/models/mcp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type RequestLogsConsumer struct {
	*JetStreamConsumer
}

//type RequestLogsBatch struct {
//	Count int                         `json:"count"` // 日志数量
//	Logs  []*AeMcpServicesRequestLogs `json:"logs"`  // 日志列表
//}

type AeMcpServicesRequestLogs struct {
	Id             int32     `json:"id"`              // 自增主键
	ServerId       string    `json:"server_id"`       // 服务id
	ToolName       string    `json:"tool_name"`       // 工具名称
	RequestTime    time.Time `json:"request_time"`    // 请求时间
	ReturnTime     time.Time `json:"return_time"`     // 返回时间
	ResponseTime   int32     `json:"response_time"`   // 响应时间(毫秒)
	Status         int16     `json:"status"`          // 请求状态：-1 失败 0 位置 1 成功
	CreateTime     time.Time `json:"create_time"`     // 创建时间
	UpdateTime     time.Time `json:"update_time"`     // 更新时间
	UserId         string    `json:"user_id"`         // 用户ID
	KeyId          int64     `json:"key_id"`          // 密钥ID
	XlcreditAmount float64   `json:"xlcredit_amount"` // 消费金额
}

// NewRequestLogsConsumer 创建消费者
func NewRequestLogsConsumer(ctx context.Context, svcCtx *svc.ServiceContext, cfg config.JetStreamConsumerConfig) *RequestLogsConsumer {
	return &RequestLogsConsumer{
		JetStreamConsumer: NewJetStreamConsumer(ctx, svcCtx, cfg),
	}
}

// Start 启动消费者
func (c *RequestLogsConsumer) Start() error {
	// 创建JetStream消费者
	if err := c.CreateOrUpdateConsumer(); err != nil {
		return err
	}

	// 开始消费
	return c.Consume(c.handleMessage)
}

func (c *RequestLogsConsumer) handleMessage(msg jetstream.Msg) {
	logx.Infof("Received message on subject [%s]: %s", msg.Subject(), string(msg.Data()))

	// 解析消息
	//var exampleMsg ExampleMessage
	var reqLog AeMcpServicesRequestLogs
	if err := json.Unmarshal(msg.Data(), &reqLog); err != nil {
		logx.Errorf("Failed to unmarshal message: %v", err)
		// 解析失败，终止该消息（不再重试）
		if err = msg.Term(); err != nil {
			logx.Errorf("Failed to term message: %v", err)
		}
		return
	}
	// 更新用户账户表数据
	//if requestLogsBatch.Count > 0 {
	var requestLogsList []*mcp.AeMcpServicesRequestLogs
	//for _, log := range requestLogsBatch.Logs {
	// 日志中消费金额大于0才会触发账户更新
	if reqLog.XlcreditAmount > 0 {
		//查询数据库中当天的用户账户数据
		userBalance, err := c.svcCtx.UserBalanceStatisticDailyModel.FindOneByDayUserId(c.ctx, reqLog.CreateTime, reqLog.UserId)
		if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
			logx.Errorf("Failed to find user balance: %v", err)
			return
		}
		if userBalance == nil {
			// 查询最后一个用户账户数据
			userBalance, err = c.svcCtx.UserBalanceStatisticDailyModel.FindOneLastDayByUserId(c.ctx, reqLog.UserId)
			if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
				logx.Errorf("Failed to find user balance: %v", err)
				return
			}
			var userBalanceTodayBalance float64 = 0
			if userBalance != nil {
				userBalanceTodayBalance = userBalance.Balance
			}
			// 创建今日用户余额
			userBalance = &fundmodel.AeUserBalanceStatisticDaily{
				UserId:  reqLog.UserId,
				Day:     reqLog.CreateTime,
				Balance: userBalanceTodayBalance,
			}
			_, err = c.svcCtx.UserBalanceStatisticDailyModel.Insert(c.ctx, userBalance)
			if err != nil {
				logx.Errorf("Failed to insert user balance: %v", err)
				return
			}
		}
		//计算扣减后的余额并存入数据库
		userBalanceStr := fmt.Sprintf("%.8f", userBalance.Balance)
		userBalanceCalculate := decimal.RequireFromString(userBalanceStr)
		logXlcreditAmountStr := fmt.Sprintf("%.8f", reqLog.XlcreditAmount)
		logXlcreditAmount := decimal.RequireFromString(logXlcreditAmountStr)
		userBalanceNewBalance, ok := userBalanceCalculate.Sub(logXlcreditAmount).Float64()
		if !ok {
			logx.Errorf("Failed to subtract user balance: %v", err)
			return
		}
		userBalance.Balance = userBalanceNewBalance
		userBalance.UpdateTime = reqLog.CreateTime
		err = c.svcCtx.UserBalanceStatisticDailyModel.Update(c.ctx, userBalance)
		if err != nil {
			logx.Errorf("Failed to update user balance: %v", err)
			return
		}
		// 同步到redis缓存
		overbalanceKey := redis.GetUserBalanceKey(reqLog.UserId)
		err = redis.SetUserBalance(c.svcCtx, overbalanceKey, userBalance.Balance, 0)
		if err != nil {
			logx.Errorf("Failed to update redis balance for user %s: %v", reqLog.UserId, err)
			return
		}
	}
	var requestLogs = mcp.AeMcpServicesRequestLogs{
		Id:           int64(reqLog.Id),
		ServerId:     reqLog.ServerId,
		ToolName:     reqLog.ToolName,
		RequestTime:  reqLog.RequestTime,
		ReturnTime:   reqLog.ReturnTime,
		ResponseTime: int64(reqLog.ResponseTime),
		Status:       int64(reqLog.Status),
		CreateTime:   reqLog.CreateTime,
		UpdateTime:   reqLog.UpdateTime,
		UserId:       reqLog.UserId,
	}
	//requestLogsList = append(requestLogsList, &requestLogs)
	//}
	//将日志存入数据库
	//err := c.svcCtx.McpServiceRequestLogsModel.InsertBatch(c.ctx, requestLogsList)
	_, err := c.svcCtx.McpServiceRequestLogsModel.Insert(c.ctx, &requestLogs)
	if err != nil {
		logx.Errorf("Failed to insert request logs: %v", err)
		logx.Infof("request logs list: %v", requestLogsList)
	}
	logx.Infof("request logs batch insert success,count: %d", len(requestLogsList))
	//}

	// 处理成功，确认消息
	if ackErr := msg.Ack(); ackErr != nil {
		logx.Errorf("Failed to ack message: %v", ackErr)
	}
}
