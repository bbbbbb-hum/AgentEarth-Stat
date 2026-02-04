package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"AgentEarth-Stat/cron/consumers"
	"AgentEarth-Stat/cron/internal/config"
	"AgentEarth-Stat/cron/internal/svc"
	"AgentEarth-Stat/cron/jobs"

	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/cron.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// 设置日志
	logx.MustSetup(c.Log)

	ctx := context.Background()
	svcCtx := svc.NewServiceContext(c)

	// 创建 cron 调度器
	cronScheduler := cron.New(cron.WithSeconds())

	// 注册定时任务

	// 注册计算响应时间平均值定时任务
	if c.Jobs.AvgResponseTimeJob.Enable {
		job := jobs.NewAvgResponseTimeJob(ctx, svcCtx)
		_, err := cronScheduler.AddFunc(c.Jobs.AvgResponseTimeJob.Cron, func() {
			job.Run()
		})
		if err != nil {
			logx.Errorf("Failed to add AvgResponseTimeJob: %v", err)
		} else {
			logx.Infof("AvgResponseTimeJob registered with cron: %s", c.Jobs.AvgResponseTimeJob.Cron)
		}
	}

	// 注册日结核销定时任务
	if c.Jobs.SettlementJob.Enable {
		job := jobs.NewSettlementJob(ctx, svcCtx)
		_, err := cronScheduler.AddFunc(c.Jobs.SettlementJob.Cron, func() {
			job.Run()
		})
		if err != nil {
			logx.Errorf("Failed to add SettlementJob: %v", err)
		} else {
			logx.Infof("SettlementJob registered with cron: %s", c.Jobs.SettlementJob.Cron)
		}
	}

	// 注册过期扣减定时任务
	if c.Jobs.ExpirationDeductionJob.Enable {
		job := jobs.NewExpirationDeductionJob(ctx, svcCtx)
		_, err := cronScheduler.AddFunc(c.Jobs.ExpirationDeductionJob.Cron, func() {
			job.Run()
		})
		if err != nil {
			logx.Errorf("Failed to add ExpirationDeductionJob: %v", err)
		} else {
			logx.Infof("ExpirationDeductionJob registered with cron: %s", c.Jobs.ExpirationDeductionJob.Cron)
		}
	}

	// 启动调度器
	cronScheduler.Start()

	// ========== 注册JetStream消费者 ==========
	var activeConsumers []consumers.Consumer

	// 注册请求日志消费者
	if c.Consumers.RequestLogsConsumer.Enable {
		consumer := consumers.NewRequestLogsConsumer(ctx, svcCtx, c.Consumers.RequestLogsConsumer)
		if err := consumer.Start(); err != nil {
			logx.Errorf("Failed to start RequestLogsConsumer: %v", err)
		} else {
			activeConsumers = append(activeConsumers, consumer)
			logx.Infof("RequestLogsConsumer started, stream: %s, subject: %s",
				c.Consumers.RequestLogsConsumer.Stream, c.Consumers.RequestLogsConsumer.Subject)
		}
	}

	// 可以继续添加更多消费者...
	// if c.Consumers.AnotherConsumer.Enable {
	//     consumer := consumers.NewAnotherConsumer(ctx, svcCtx, c.Consumers.AnotherConsumer)
	//     ...
	// }

	fmt.Println("Cron service started...")

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logx.Info("Shutting down cron service...")

	// 停止定时任务
	cronScheduler.Stop()

	// 停止所有消费者
	for _, consumer := range activeConsumers {
		if err := consumer.Stop(); err != nil {
			logx.Errorf("Failed to stop consumer: %v", err)
		}
	}

	// 关闭服务上下文（包括NATS连接）
	svcCtx.Close()

	logx.Info("Cron service stopped")
}
