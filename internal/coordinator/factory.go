package coordinator

import (
	"context"
	"log"
	"os"
	"time"
)

// EnvRedisURL 是 Redis 连接串环境变量名。
// 设置后启用 Redis Coordinator,未设置或连接失败则使用内存实现。
const EnvRedisURL = "MAXX_REDIS_URL"

// FromEnv 根据环境变量构造 Coordinator。
//   - MAXX_REDIS_URL 未设置 → 直接使用内存实现
//   - MAXX_REDIS_URL 已设置但连接失败 → 警告后退化为内存实现,不阻塞启动
//
// 返回的 Coordinator 一定非空。
func FromEnv(ctx context.Context, instanceID string) Coordinator {
	url := os.Getenv(EnvRedisURL)
	if url == "" {
		log.Printf("[Coordinator] using in-memory implementation (set %s to enable Redis)", EnvRedisURL)
		return NewMemory(instanceID)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	c, err := NewRedis(pingCtx, url, instanceID)
	if err != nil {
		log.Printf("[Coordinator] WARNING: Redis unavailable (%v), falling back to in-memory. Multi-instance coordination will NOT work.", err)
		return NewMemory(instanceID)
	}
	log.Printf("[Coordinator] connected to Redis as instance=%s", instanceID)
	return c
}
