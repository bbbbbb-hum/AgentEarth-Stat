package redis

import (
	"AgentEarth-Stat/cron/internal/svc"
	"fmt"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// ScanPrevHourUserUsageIncrementKeys 扫描Redis中上一个小时所有用户的使用量增量key
// 返回匹配的key列表
func ScanPrevHourUserUsageIncrementKeys(sc *svc.ServiceContext) ([]string, error) {
	redisConfig := NewRedisConfig(sc)
	prevHour := time.Now().Add(-time.Hour).Format("2006-01-02-15")
	pattern := redisConfig.BuildKey("user_usage_increment", prevHour, "*")

	var allKeys []string
	var cursor uint64 = 0
	for {
		keys, cur, err := sc.Redis.Scan(cursor, pattern, 100)
		if err != nil {
			return nil, fmt.Errorf("scan redis keys failed: %w", err)
		}
		allKeys = append(allKeys, keys...)
		cursor = cur
		if cursor == 0 {
			break
		}
	}

	logx.Infof("Scanned %d user usage increment keys for previous hour, pattern: %s", len(allKeys), pattern)
	return allKeys, nil
}

// ExtractUserIdFromKey 从key中提取userId
// key格式: {prefix}:user_usage_increment:{hour}:{userId}
func ExtractUserIdFromKey(key string) string {
	parts := strings.Split(key, ":")
	if len(parts) < 1 {
		return ""
	}
	return parts[len(parts)-1]
}

// ExtractDateFromKey 从key中提取日期（天）
// key格式: {prefix}:user_usage_increment:{YYYY-MM-DD-HH}:{userId}
// 返回格式: YYYY-MM-DD
func ExtractDateFromKey(key string) string {
	parts := strings.Split(key, ":")
	if len(parts) < 3 {
		return ""
	}
	// 倒数第二段是 {YYYY-MM-DD-HH}，取前10个字符即 YYYY-MM-DD
	hourPart := parts[len(parts)-2]
	if len(hourPart) >= 10 {
		return hourPart[:10]
	}
	return ""
}

// GetValueByKey 根据key获取值
func GetValueByKey(sc *svc.ServiceContext, key string) (float64, error) {
	val, err := sc.Redis.Get(key)
	if err != nil {
		return 0, err
	}

	var value float64
	_, err = fmt.Sscanf(val, "%f", &value)
	if err != nil {
		return 0, fmt.Errorf("parse value failed: %w", err)
	}

	return value, nil
}

// DeleteKey 删除指定key
func DeleteKey(sc *svc.ServiceContext, key string) error {
	_, err := sc.Redis.Del(key)
	return err
}
