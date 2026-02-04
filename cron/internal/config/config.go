package config

import (
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type Config struct {
	Log   logx.LogConf
	Redis redis.RedisConf
	DB    struct {
		DataSource string
	}
	Nats NatsConfig
	Jobs struct {
		AvgResponseTimeJob struct {
			Enable bool
			Cron   string
		}
	}
	Consumers ConsumersConfig
}

// NatsConfig NATS连接配置
type NatsConfig struct {
	Url      string
	Username string `json:",optional"`
	Password string `json:",optional"`
	Token    string `json:",optional"`
}

// ConsumersConfig 消费者配置
type ConsumersConfig struct {
	//ExampleConsumer     JetStreamConsumerConfig
	RequestLogsConsumer JetStreamConsumerConfig
}

// JetStreamConsumerConfig JetStream消费者配置
type JetStreamConsumerConfig struct {
	Enable        bool
	Stream        string // JetStream Stream名称
	Subject       string // 订阅的主题
	Durable       string // 持久化消费者名称
	AckWait       int    `json:",optional,default=30"`   // 消息确认超时时间（秒）
	MaxDeliver    int    `json:",optional,default=3"`    // 消息最大重试次数
	MaxAckPending int    `json:",optional,default=1000"` // 最大未确认消息数
}
