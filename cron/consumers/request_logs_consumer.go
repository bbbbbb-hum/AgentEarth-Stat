package consumers

import (
	"AgentEarth-Stat/cron/internal/config"
	"AgentEarth-Stat/cron/internal/redis"
	"AgentEarth-Stat/cron/internal/svc"
	"AgentEarth-Stat/models/mcp"
	"AgentEarth-Stat/models/users"
	"context"
	"encoding/json"
	"errors"
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
	var reqLog AeMcpServicesRequestLogs
	if err := json.Unmarshal(msg.Data(), &reqLog); err != nil {
		logx.Errorf("Failed to unmarshal message: %v", err)
		// 解析失败，终止该消息（不再重试，因为消息格式错误重试也没用）
		if termErr := msg.Term(); termErr != nil {
			logx.Errorf("Failed to term message: %v", termErr)
		}
		return
	}
	logx.Infof("Received message: %+v", reqLog)
	// 用于记录事务中更新后的余额，用于后续同步Redis
	var updatedBalance float64
	var needSyncRedis bool

	// 使用事务处理：更新用户账户 + 插入日志
	err := c.svcCtx.DB.TransactCtx(c.ctx, func(ctx context.Context, session sqlx.Session) error {
		// 在事务中创建 Model 实例
		userBalanceModel := users.NewAeUserBalanceStatisticDailyModel(sqlx.NewSqlConnFromSession(session))
		requestLogsModel := mcp.NewAeMcpServicesRequestLogsModel(sqlx.NewSqlConnFromSession(session))

		// 日志中消费金额大于0才会触发账户更新
		if reqLog.XlcreditAmount > 0 {
			// 查询数据库中当天的用户账户数据
			userBalance, err := userBalanceModel.FindOneByDayUserId(ctx, reqLog.CreateTime, reqLog.UserId)
			if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
				return err
			}
			if userBalance == nil {
				// 查询最后一个用户账户数据
				userBalance, err = userBalanceModel.FindOneLastDayByUserId(ctx, reqLog.UserId)
				if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
					return err
				}
				var userBalanceTodayBalance float64 = 0
				if userBalance != nil {
					userBalanceTodayBalance = userBalance.Balance
				}
				// 创建今日用户余额
				userBalance = &users.AeUserBalanceStatisticDaily{
					UserId:  reqLog.UserId,
					Day:     reqLog.CreateTime,
					Balance: userBalanceTodayBalance,
				}
				_, err = userBalanceModel.Insert(ctx, userBalance)
				if err != nil {
					return err
				}
			}

			// 计算扣减后的余额并存入数据库
			userBalanceCalculate := decimal.NewFromFloat(userBalance.Balance)
			logXlcreditAmount := decimal.NewFromFloat(reqLog.XlcreditAmount)
			userBalanceNewBalance, _ := userBalanceCalculate.Sub(logXlcreditAmount).Float64()

			userBalance.Balance = userBalanceNewBalance
			userBalance.UpdateTime = reqLog.CreateTime
			err = userBalanceModel.Update(ctx, userBalance)
			if err != nil {
				return err
			}

			// 记录更新后的余额，用于事务成功后同步Redis
			updatedBalance = userBalanceNewBalance
			needSyncRedis = true
		}

		// 插入请求日志
		requestLogs := mcp.AeMcpServicesRequestLogs{
			ServerId:       reqLog.ServerId,
			ToolName:       reqLog.ToolName,
			RequestTime:    reqLog.RequestTime,
			ReturnTime:     reqLog.ReturnTime,
			ResponseTime:   int64(reqLog.ResponseTime),
			Status:         int64(reqLog.Status),
			CreateTime:     reqLog.CreateTime,
			UpdateTime:     reqLog.UpdateTime,
			UserId:         reqLog.UserId,
			KeyId:          reqLog.KeyId,
			XlcreditAmount: reqLog.XlcreditAmount,
		}
		_, err := requestLogsModel.Insert(ctx, &requestLogs)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		logx.Errorf("Transaction failed for user %s: %v", reqLog.UserId, err)
		// 事务失败，延迟重试
		c.nakWithDelay(msg)
		return
	}

	logx.Infof("Transaction success for user: %s, server: %s", reqLog.UserId, reqLog.ServerId)

	// 事务成功后同步Redis（失败只记录日志，不影响消息确认）
	if needSyncRedis {
		err = redis.SetUserBalance(c.svcCtx, reqLog.UserId, updatedBalance, 0)
		if err != nil {
			logx.Errorf("Failed to sync redis balance for user %s: %v", reqLog.UserId, err)
		}
	}

	// 处理成功，确认消息
	if ackErr := msg.Ack(); ackErr != nil {
		logx.Errorf("Failed to ack message: %v", ackErr)
	}
}

// nakWithDelay 处理失败时延迟重试
func (c *RequestLogsConsumer) nakWithDelay(msg jetstream.Msg) {
	if err := msg.NakWithDelay(5 * time.Second); err != nil {
		logx.Errorf("Failed to nak message with delay: %v", err)
	}
}
