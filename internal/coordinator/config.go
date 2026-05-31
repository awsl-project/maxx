package coordinator

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// Mode 决定 Redis 不可用时的行为。三种模式互斥,启动时确定后不变。
type Mode string

const (
	// ModeStandalone 不使用 Redis,内存实现。MAXX_COORDINATOR_REDIS_URL 若被设置
	// 会被忽略并发出 warning。默认模式。
	ModeStandalone Mode = "standalone"

	// ModeFailFast 要求 Redis 必须连通,否则启动失败。生产多实例部署应使用此模式。
	ModeFailFast Mode = "fail-fast"

	// ModeDegraded 启动时优先尝试 Redis;失败时退化为内存实现,后台尝试重连。
	// 重连成功后调用 OnUpgrade 回调把进程升级到 Redis。
	// 注意:仅做一次性 mem → redis 恢复,不做反复抖动。
	ModeDegraded Mode = "degraded"
)

// Config 是 coordinator 启动配置。所有字段都可以由环境变量初始化,
// 也允许调用方手动构造(便于测试与 desktop launcher)。
type Config struct {
	Mode              Mode
	RedisURL          string
	InstanceTTL       time.Duration
	HeartbeatInterval time.Duration
	ReconnectInterval time.Duration
	SweepInterval     time.Duration // 周期性 stale-request sweep 间隔
}

// 环境变量名常量,与 docs/multi-instance-rfc.md 保持一致。
const (
	EnvMode              = "MAXX_COORDINATOR_MODE"
	EnvRedisURLPrimary   = "MAXX_COORDINATOR_REDIS_URL"
	EnvRedisURLLegacy    = "MAXX_REDIS_URL" // 保留兼容
	EnvInstanceTTL       = "MAXX_COORDINATOR_INSTANCE_TTL"
	EnvHeartbeatInterval = "MAXX_COORDINATOR_HEARTBEAT_INTERVAL"
	EnvReconnectInterval = "MAXX_COORDINATOR_RECONNECT_INTERVAL"
	EnvSweepInterval     = "MAXX_PROXY_REQUEST_SWEEP_INTERVAL"
)

// ConfigFromEnv 从环境变量加载配置。未设置的字段使用 RFC 默认值。
// 返回的 Config 仍需通过 Validate 才能用于构造 coordinator。
//
// 模式默认值的智能选择:
//   - MAXX_COORDINATOR_MODE 显式设置 → 用它
//   - 未设置 + 没有任何 Redis URL → standalone
//   - 未设置 + 有 Redis URL → degraded(向后兼容老的 MAXX_REDIS_URL 行为:
//     启动时连不上不致命,但运行中明确警告 + 后台重连)
//
// 想要"配 URL 但允许 standalone 退化(原 silent fallback 行为)"已不再支持。
// 想要严格模式请显式设 MAXX_COORDINATOR_MODE=fail-fast。
func ConfigFromEnv() Config {
	c := Config{
		InstanceTTL:       60 * time.Second,
		HeartbeatInterval: 20 * time.Second,
		ReconnectInterval: 5 * time.Second,
		SweepInterval:     45 * time.Second,
	}

	c.RedisURL = strings.TrimSpace(os.Getenv(EnvRedisURLPrimary))
	usedLegacyURL := false
	if c.RedisURL == "" {
		if legacy := strings.TrimSpace(os.Getenv(EnvRedisURLLegacy)); legacy != "" {
			c.RedisURL = legacy
			usedLegacyURL = true
		}
	}

	if v := strings.TrimSpace(os.Getenv(EnvMode)); v != "" {
		c.Mode = Mode(strings.ToLower(v))
	} else if c.RedisURL != "" {
		// 老用户设置过 MAXX_REDIS_URL,默认走 degraded(连得上就用,连不上 warn)
		c.Mode = ModeDegraded
		if usedLegacyURL {
			log.Printf("[Coordinator] DEPRECATION: %s is set but %s is not — "+
				"defaulting to %s mode. Set %s=%s (or =fail-fast/standalone) "+
				"explicitly and use %s to silence this warning.",
				EnvRedisURLLegacy, EnvRedisURLPrimary,
				ModeDegraded, EnvMode, ModeDegraded, EnvRedisURLPrimary)
		}
	} else {
		c.Mode = ModeStandalone
	}

	if d, ok := parseDurationEnv(EnvInstanceTTL); ok {
		c.InstanceTTL = d
	}
	if d, ok := parseDurationEnv(EnvHeartbeatInterval); ok {
		c.HeartbeatInterval = d
	}
	if d, ok := parseDurationEnv(EnvReconnectInterval); ok {
		c.ReconnectInterval = d
	}
	if d, ok := parseDurationEnv(EnvSweepInterval); ok {
		c.SweepInterval = d
	}

	return c
}

func parseDurationEnv(key string) (time.Duration, bool) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0, false
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

// Validate 检查配置的合法性。当 mode 显式要求 Redis(fail-fast/degraded)但
// URL 缺失时返回错误,这是 fail-fast 行为的一部分:错误配置在启动期暴露,
// 不要静默退化。
func (c *Config) Validate() error {
	switch c.Mode {
	case ModeStandalone, ModeFailFast, ModeDegraded:
		// ok
	default:
		return fmt.Errorf("invalid %s=%q (want standalone, fail-fast, or degraded)", EnvMode, c.Mode)
	}

	if c.Mode != ModeStandalone && c.RedisURL == "" {
		return fmt.Errorf("%s=%s requires %s to be set", EnvMode, c.Mode, EnvRedisURLPrimary)
	}

	if c.InstanceTTL <= 0 {
		return fmt.Errorf("InstanceTTL must be > 0")
	}
	if c.HeartbeatInterval <= 0 {
		return fmt.Errorf("HeartbeatInterval must be > 0")
	}
	if c.HeartbeatInterval >= c.InstanceTTL {
		return fmt.Errorf("HeartbeatInterval (%v) must be shorter than InstanceTTL (%v)", c.HeartbeatInterval, c.InstanceTTL)
	}
	if c.ReconnectInterval <= 0 {
		return fmt.Errorf("ReconnectInterval must be > 0")
	}
	if c.SweepInterval <= 0 {
		return fmt.Errorf("SweepInterval must be > 0")
	}

	return nil
}
