package coordinator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// EnvRedisURL is retained for backward compatibility with the original draft.
// New code should reference EnvRedisURLPrimary.
//
// Deprecated: use EnvRedisURLPrimary.
const EnvRedisURL = EnvRedisURLLegacy

// ErrFatalConfig 表示配置或连接错误已经达到不能继续启动的程度。
// 调用方应直接 log.Fatal 或返回非零退出码。
var ErrFatalConfig = errors.New("coordinator: fatal configuration error")

// Build 按 Config 构造一个 Coordinator。三态行为见 Mode 注释。
//
// 返回的 cleanup 函数在调用方退出时使用,通常 main.go 的 shutdown 阶段调用:
//   - 内存模式下只是 Close()
//   - Redis 模式下会 Unregister 实例 + Close 客户端
//   - degraded + 重连 goroutine 模式下会取消重连 ctx
//
// 错误返回值 wrap ErrFatalConfig 表示"不要继续启动":
//   - mode=fail-fast 但 Redis 拨号失败
//   - 配置无效(Validate 失败)
func Build(ctx context.Context, cfg Config, instanceID string) (Coordinator, func(), error) {
	if err := cfg.Validate(); err != nil {
		return nil, func() {}, fmt.Errorf("%w: %v", ErrFatalConfig, err)
	}

	switch cfg.Mode {
	case ModeStandalone:
		if cfg.RedisURL != "" {
			log.Printf("[Coordinator] mode=standalone, ignoring %s (set %s=fail-fast or =degraded to enable distributed mode)",
				EnvRedisURLPrimary, EnvMode)
		}
		log.Printf("[Coordinator] running in standalone mode (in-process memory coordinator)")
		c := NewMemory(instanceID)
		return c, func() { _ = c.Close() }, nil

	case ModeFailFast:
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		c, err := NewRedis(pingCtx, cfg.RedisURL, instanceID)
		if err != nil {
			return nil, func() {}, fmt.Errorf("%w: %s=fail-fast requires Redis to be reachable: %v",
				ErrFatalConfig, EnvMode, err)
		}
		log.Printf("[Coordinator] running in distributed mode (fail-fast), connected to Redis as instance=%s", instanceID)
		return c, func() { _ = c.Close() }, nil

	case ModeDegraded:
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		c, err := NewRedis(pingCtx, cfg.RedisURL, instanceID)
		cancel()
		if err == nil {
			log.Printf("[Coordinator] running in distributed mode (degraded-capable), connected to Redis as instance=%s", instanceID)
			return c, func() { _ = c.Close() }, nil
		}
		log.Printf("[Coordinator] WARNING: degraded mode falling back to memory (Redis unreachable: %v). Multi-instance coordination is NOT active.", err)
		mem := NewMemory(instanceID)
		reconnectCtx, reconnectCancel := context.WithCancel(ctx)
		go watchForRedisRecovery(reconnectCtx, cfg)
		cleanup := func() {
			reconnectCancel()
			_ = mem.Close()
		}
		return mem, cleanup, nil
	}

	return nil, func() {}, fmt.Errorf("%w: unreachable mode %q", ErrFatalConfig, cfg.Mode)
}

// watchForRedisRecovery 周期性 PING Redis;一旦恢复就打 WARN 提示重启。
// v1 不做运行时热切换 —— 见 RFC 中 "degraded recovery in v1 is limited
// to a startup-time grace window"。
func watchForRedisRecovery(ctx context.Context, cfg Config) {
	ticker := time.NewTicker(cfg.ReconnectInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			c, err := NewRedis(pingCtx, cfg.RedisURL, "ping")
			cancel()
			if err != nil {
				continue
			}
			_ = c.Close()
			log.Printf("[Coordinator] NOTICE: Redis is now reachable. Restart the process to enable distributed coordination.")
			return // 提示一次就退出,不再重复打扰
		}
	}
}

// FromEnv is the legacy single-call entrypoint. New code should compose
// ConfigFromEnv + Build directly so callers can see all errors. Kept here
// for callers that haven't been updated yet; logs a fatal-equivalent and
// returns a memory coordinator only when mode resolves to standalone.
//
// Deprecated: use ConfigFromEnv() + Build(ctx, cfg, instanceID).
func FromEnv(ctx context.Context, instanceID string) Coordinator {
	cfg := ConfigFromEnv()
	c, _, err := Build(ctx, cfg, instanceID)
	if err != nil {
		log.Fatalf("[Coordinator] %v", err)
	}
	return c
}
