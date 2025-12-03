package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
		// 立即执行一次
		//job.Run()
	}

	// 启动调度器
	cronScheduler.Start()
	fmt.Println("Cron service started...")

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logx.Info("Shutting down cron service...")
	cronScheduler.Stop()
	logx.Info("Cron service stopped")
}
