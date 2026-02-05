package consumers

import (
	"context"
	"time"

	"AgentEarth-Stat/cron/internal/config"
	"AgentEarth-Stat/cron/internal/svc"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/zeromicro/go-zero/core/logx"
)

// Consumer 消费者接口
type Consumer interface {
	Start() error
	Stop() error
}

// JetStreamConsumer JetStream消费者基础结构
type JetStreamConsumer struct {
	ctx      context.Context
	cancel   context.CancelFunc
	svcCtx   *svc.ServiceContext
	config   config.JetStreamConsumerConfig
	consumer jetstream.Consumer
	stopCh   chan struct{}
}

// NewJetStreamConsumer 创建JetStream消费者
func NewJetStreamConsumer(ctx context.Context, svcCtx *svc.ServiceContext, cfg config.JetStreamConsumerConfig) *JetStreamConsumer {
	consumerCtx, cancel := context.WithCancel(ctx)
	return &JetStreamConsumer{
		ctx:    consumerCtx,
		cancel: cancel,
		svcCtx: svcCtx,
		config: cfg,
		stopCh: make(chan struct{}),
	}
}

// CreateOrUpdateConsumer 创建或更新JetStream消费者
func (c *JetStreamConsumer) CreateOrUpdateConsumer() error {
	if c.svcCtx.JetStream == nil {
		logx.Error("JetStream is nil, cannot create consumer")
		return nil
	}

	// 获取或创建Stream
	stream, err := c.svcCtx.JetStream.Stream(c.ctx, c.config.Stream)
	if err != nil {
		logx.Errorf("Failed to get stream %s: %v", c.config.Stream, err)
		return err
	}

	// 创建消费者配置
	consumerCfg := jetstream.ConsumerConfig{
		Durable:       c.config.Durable,
		FilterSubject: c.config.Subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       time.Duration(c.config.AckWait) * time.Second,
		MaxDeliver:    c.config.MaxDeliver,
		MaxAckPending: c.config.MaxAckPending,
	}

	// 创建或更新消费者
	consumer, err := stream.CreateOrUpdateConsumer(c.ctx, consumerCfg)
	if err != nil {
		logx.Errorf("Failed to create consumer %s: %v", c.config.Durable, err)
		return err
	}

	c.consumer = consumer
	logx.Infof("JetStream consumer created: stream=%s, durable=%s, subject=%s",
		c.config.Stream, c.config.Durable, c.config.Subject)
	return nil
}

// Consume 开始消费消息
func (c *JetStreamConsumer) Consume(handler func(msg jetstream.Msg)) error {
	if c.consumer == nil {
		logx.Error("Consumer is nil, call CreateOrUpdateConsumer first")
		return nil
	}

	// 使用Consume方法持续消费消息
	cons, err := c.consumer.Consume(func(msg jetstream.Msg) {
		handler(msg)
	})
	if err != nil {
		logx.Errorf("Failed to start consuming: %v", err)
		return err
	}

	logx.Infof("Started consuming from stream=%s, subject=%s", c.config.Stream, c.config.Subject)

	// 等待停止信号
	go func() {
		<-c.stopCh
		cons.Stop()
		logx.Info("Consumer stopped")
	}()

	return nil
}

// Stop 停止消费者
func (c *JetStreamConsumer) Stop() error {
	logx.Info("Stopping JetStream consumer...")
	c.cancel()
	close(c.stopCh)
	return nil
}

// GetContext 获取上下文
func (c *JetStreamConsumer) GetContext() context.Context {
	return c.ctx
}

// GetServiceContext 获取服务上下文
func (c *JetStreamConsumer) GetServiceContext() *svc.ServiceContext {
	return c.svcCtx
}

// GetConfig 获取配置
func (c *JetStreamConsumer) GetConfig() config.JetStreamConsumerConfig {
	return c.config
}
