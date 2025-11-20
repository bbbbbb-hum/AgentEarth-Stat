package jobs

import (
	"AgentEarth-Stat/cron/internal/svc"
	"context"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

type ExampleJob struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewExampleJob(ctx context.Context, svcCtx *svc.ServiceContext) *ExampleJob {
	return &ExampleJob{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (j *ExampleJob) Run() {
	j.Infof("ExampleJob started at %s", time.Now().Format("2006-01-02 15:04:05"))

	// 在这里编写你的定时任务逻辑
	// 例如：数据清理、报表生成、数据同步等

	j.Info("ExampleJob completed")
}
