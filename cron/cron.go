package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
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

	// 启动 HTTP 服务
	go func() {
		handlers := &Handlers{svcCtx: svcCtx}
		addr := c.Host + ":" + fmt.Sprintf("%d", c.Port)
		http.HandleFunc("/health", handlers.healthHandler)
		http.HandleFunc("/ready", handlers.readyHandler)
		logx.Infof("HTTP server started on :%s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			logx.Errorf("Failed to start HTTP server: %v", err)
		}
	}()

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
	// 注册用户余额定时任务
	if c.Jobs.UserBalanceJob.Enable {
		job := jobs.NewUserBalanceJob(ctx, svcCtx)
		_, err := cronScheduler.AddFunc(c.Jobs.UserBalanceJob.Cron, func() {
			job.Run()
		})
		if err != nil {
			logx.Errorf("Failed to add UserBalanceJob: %v", err)
		}
		logx.Infof("UserBalanceJob registered with cron: %s", c.Jobs.UserBalanceJob.Cron)
	}

	// 启动调度器
	cronScheduler.Start()

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

	logx.Info("Cron service stopped")
}

type Handlers struct {
	svcCtx *svc.ServiceContext
}

func (h *Handlers) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "healthy"}`)
}

func (h *Handlers) readyHandler(w http.ResponseWriter, r *http.Request) {
	if h.svcCtx.IsReady() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status": "ready"}`)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status": "not ready"}`)
	}
}
