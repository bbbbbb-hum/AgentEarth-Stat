package redis

import (
	"AgentEarth-Stat/cron/internal/svc"
	"strings"
)

type RedisConfig struct {
	Svc *svc.ServiceContext
}

func NewRedisConfig(svc *svc.ServiceContext) *RedisConfig {
	return &RedisConfig{
		Svc: svc,
	}
}
func (c *RedisConfig) BuildKey(parts ...string) string {
	//clientMu.RLock()
	prefix := c.Svc.Config.NameSpace + "_ae"
	//clientMu.RUnlock()

	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		cleanParts = append(cleanParts, strings.Trim(part, ":"))
	}

	if prefix == "" {
		return strings.Join(cleanParts, ":")
	}
	if len(cleanParts) == 0 {
		return prefix
	}
	return prefix + ":" + strings.Join(cleanParts, ":")
}
