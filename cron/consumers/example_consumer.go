package consumers

import (
	"context"
	"encoding/json"

	"AgentEarth-Stat/cron/internal/config"
	"AgentEarth-Stat/cron/internal/svc"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/zeromicro/go-zero/core/logx"
)

// ExampleConsumer 示例消费者
type ExampleConsumer struct {
	*JetStreamConsumer
}

// ExampleMessage 示例消息结构
type ExampleMessage struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Payload any    `json:"payload"`
}

// NewExampleConsumer 创建示例消费者
func NewExampleConsumer(ctx context.Context, svcCtx *svc.ServiceContext, cfg config.JetStreamConsumerConfig) *ExampleConsumer {
	return &ExampleConsumer{
		JetStreamConsumer: NewJetStreamConsumer(ctx, svcCtx, cfg),
	}
}

// Start 启动消费者
func (c *ExampleConsumer) Start() error {
	// 创建JetStream消费者
	if err := c.CreateOrUpdateConsumer(); err != nil {
		return err
	}

	// 开始消费
	return c.Consume(c.handleMessage)
}

// handleMessage 处理消息
func (c *ExampleConsumer) handleMessage(msg jetstream.Msg) {
	logx.Infof("Received message on subject [%s]: %s", msg.Subject(), string(msg.Data()))

	// 解析消息
	var exampleMsg ExampleMessage
	if err := json.Unmarshal(msg.Data(), &exampleMsg); err != nil {
		logx.Errorf("Failed to unmarshal message: %v", err)
		// 解析失败，终止该消息（不再重试）
		if err := msg.Term(); err != nil {
			logx.Errorf("Failed to term message: %v", err)
		}
		return
	}

	// 根据action处理不同的业务逻辑
	var err error
	switch exampleMsg.Action {
	case "create":
		err = c.handleCreate(exampleMsg)
	case "update":
		err = c.handleUpdate(exampleMsg)
	case "delete":
		err = c.handleDelete(exampleMsg)
	default:
		logx.Infof("[WARN] Unknown action: %s", exampleMsg.Action)
		// 未知action，终止该消息
		if termErr := msg.Term(); termErr != nil {
			logx.Errorf("Failed to term message: %v", termErr)
		}
		return
	}

	// 根据处理结果确认或重试
	if err != nil {
		logx.Errorf("Failed to handle message: %v", err)
		// 处理失败，触发重试（Nak会让消息稍后重新投递）
		if nakErr := msg.Nak(); nakErr != nil {
			logx.Errorf("Failed to nak message: %v", nakErr)
		}
		return
	}

	// 处理成功，确认消息
	if ackErr := msg.Ack(); ackErr != nil {
		logx.Errorf("Failed to ack message: %v", ackErr)
	}
}

// handleCreate 处理创建操作
func (c *ExampleConsumer) handleCreate(msg ExampleMessage) error {
	logx.Infof("Handling create action for ID: %s", msg.ID)
	// TODO: 实现你的业务逻辑
	// 例如：c.GetServiceContext().McpServiceModel.Insert(...)
	return nil
}

// handleUpdate 处理更新操作
func (c *ExampleConsumer) handleUpdate(msg ExampleMessage) error {
	logx.Infof("Handling update action for ID: %s", msg.ID)
	// TODO: 实现你的业务逻辑
	return nil
}

// handleDelete 处理删除操作
func (c *ExampleConsumer) handleDelete(msg ExampleMessage) error {
	logx.Infof("Handling delete action for ID: %s", msg.ID)
	// TODO: 实现你的业务逻辑
	return nil
}
