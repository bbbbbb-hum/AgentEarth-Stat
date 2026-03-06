package config

import (
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	rest.RestConf
	//Log   logx.LogConf `json:"Log"`
	NameSpace string
	Redis     redis.RedisConf
	DB        struct {
		DataSource string
	}
	Jobs struct {
		AvgResponseTimeJob struct {
			Enable bool
			Cron   string
		}
		UserBalanceJob struct {
			Enable bool
			Cron   string
		}
	}
}
