package coordinator

import (
	"context"
	"log"
	"time"
)

// StartHeartbeat 启动一个后台心跳 goroutine。
// 立即调用一次 RegisterInstance,之后每 ttl/3 续期一次。
// ctx 取消时停止;调用方在优雅退出时另外调用 UnregisterInstance 立即下线。
func StartHeartbeat(ctx context.Context, c Coordinator, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	interval := ttl / 3
	if interval < time.Second {
		interval = time.Second
	}

	if err := c.RegisterInstance(ctx, ttl); err != nil {
		log.Printf("[Coordinator] initial RegisterInstance failed: %v", err)
	}

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := c.RefreshInstance(ctx, ttl); err != nil {
					log.Printf("[Coordinator] RefreshInstance failed: %v", err)
				}
			}
		}
	}()
}
