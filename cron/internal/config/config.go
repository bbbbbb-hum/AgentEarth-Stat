package config

import "github.com/zeromicro/go-zero/core/logx"

type Config struct {
	Log logx.LogConf
	DB  struct {
		DataSource string
	}
	Jobs struct {
		AvgResponseTimeJob struct {
			Enable bool
			Cron   string
		}
	}
}
