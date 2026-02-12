// Package testapi 提供用于 Apifox/本地触发定时任务的 HTTP 接口，仅测试用，生产请关闭。
package testapi

import (
	"AgentEarth-Stat/cron/internal/config"
	"AgentEarth-Stat/cron/internal/svc"
	"AgentEarth-Stat/cron/jobs"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/zeromicro/go-zero/core/logx"
)

// RunJobResponse 响应体
type RunJobResponse struct {
	OK  bool   `json:"ok"`
	Job string `json:"job,omitempty"`
	Msg string `json:"msg,omitempty"`
}

// TODO: 启动测试 API 服务，阻塞调用；应在 goroutine 中调用。
func Start(c config.Config, svcCtx *svc.ServiceContext, ctx context.Context) {
	if !c.TestAPI.Enable {
		return
	}
	port := c.TestAPI.Port
	if port <= 0 {
		port = 9090
	}
	mux := http.NewServeMux()
	// 两个独立接口，可分别测试，无需等待
	//TODO: 生产环境安全考虑，暂时关闭以下测试接口；需要测试时手动恢复注释即可。
	//mux.HandleFunc("/test/run-settlement", handleRunSettlement(svcCtx, ctx))
	//mux.HandleFunc("/test/run-expiration", handleRunExpiration(svcCtx, ctx))
	addr := fmt.Sprintf(":%d", port)
	logx.Infof("[TestAPI] 监听 %s，仅用于测试，生产请关闭 TestAPI.Enable", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logx.Errorf("[TestAPI] 服务异常: %v", err)
	}
}

// TODO:测试用，用于 Apifox 触发定时任务
func handleRunSettlement(svcCtx *svc.ServiceContext, ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, RunJobResponse{OK: false, Msg: "需要 POST"})
			return
		}
		job := jobs.NewSettlementJob(ctx, svcCtx)
		job.Run()
		writeJSON(w, http.StatusOK, RunJobResponse{OK: true, Job: "settlement", Msg: "已执行"})
	}
}

func handleRunExpiration(svcCtx *svc.ServiceContext, ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, RunJobResponse{OK: false, Msg: "需要 POST"})
			return
		}
		job := jobs.NewExpirationDeductionJob(ctx, svcCtx)
		job.Run()
		writeJSON(w, http.StatusOK, RunJobResponse{OK: true, Job: "expiration", Msg: "已执行"})
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
