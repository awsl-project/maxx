package cached

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
)

// publish 通过 coordinator 投递一条消息,订阅者能收到
func TestCacheBroadcastPublishReachesSubscriber(t *testing.T) {
	t.Parallel()
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := coord.Subscribe(ctx, cacheChannelPrefix+"x")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	bc := &cacheBroadcast{}
	bc.attach(coord, "x")
	bc.publish(OpUpdate, 42)

	select {
	case msg := <-ch:
		if msg.Sender != "inst-A" {
			t.Fatalf("sender = %q, want inst-A", msg.Sender)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive published invalidation")
	}
}

// AttachInvalidation 过滤掉本实例自己发的事件,避免清掉刚写好的本地缓存
func TestAttachInvalidationFiltersSelf(t *testing.T) {
	t.Parallel()
	coord := coordinator.NewMemory("inst-A")
	defer coord.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var hits int32
	AttachInvalidation(ctx, coord, "x", func() { atomic.AddInt32(&hits, 1) })

	// 同一 coordinator publish:Sender == inst-A,订阅者应过滤
	bc := &cacheBroadcast{}
	bc.attach(coord, "x")
	bc.publish(OpUpdate, 42)

	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Fatalf("self-sent invalidation should be filtered, got %d hits", got)
	}
}

func TestCacheBroadcastNilCoordIsNoop(t *testing.T) {
	t.Parallel()
	bc := &cacheBroadcast{}
	bc.publish(OpUpdate, 1) // 不应 panic
}

func TestAttachInvalidationNilCoordIsNoop(t *testing.T) {
	t.Parallel()
	AttachInvalidation(context.Background(), nil, "x", func() {
		t.Fatal("callback must not run for nil coordinator")
	})
}
