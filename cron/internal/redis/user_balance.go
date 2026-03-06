package redis

import (
	"AgentEarth-Stat/cron/internal/svc"
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// 获取用户余额key
func GetUserBalanceKey(sc *svc.ServiceContext, userId string) string {
	redisConfig := NewRedisConfig(sc)
	today := time.Now().Format("2006-01-02")
	return redisConfig.BuildKey("user_balance", userId, today)
}

// GetUserBalance 从Redis获取用户余额
func GetUserBalance(sc *svc.ServiceContext, userId string) (float64, bool, error) {
	key := GetUserBalanceKey(sc, userId)

	// 检查key是否存在
	exists, err := sc.Redis.Exists(key)
	if err != nil {
		return 0, false, err
	}
	if !exists {
		return 0, false, nil
	}

	// 获取值
	val, err := sc.Redis.Get(key)
	if err != nil {
		return 0, false, err
	}

	var balance float64
	_, err = fmt.Sscanf(val, "%f", &balance)
	if err != nil {
		return 0, false, err
	}

	return balance, true, nil
}

// SetUserBalance 设置用户余额到Redis
func SetUserBalance(sc *svc.ServiceContext, userId string, balance float64, expireSeconds int) error {
	key := GetUserBalanceKey(sc, userId)
	val := fmt.Sprintf("%.8f", balance)
	if expireSeconds > 0 {
		return sc.Redis.Setex(key, val, expireSeconds)
	}
	logx.Infof("设置用户余额到Redis: %s, %s", key, val)
	// 默认存储24小时
	return sc.Redis.Setex(key, val, 60*60*24)
}
