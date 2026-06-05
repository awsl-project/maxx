package handler

import (
	"context"
	"testing"
	"time"
)

func TestUploadLimiterGatesOnlyLargeRequests(t *testing.T) {
	// 阈值 1KiB,并发上限 2,无硬上限,不等待。
	l := newUploadLimiter(1<<10, 2, 0, 0)

	// 小请求永不门控,且不占名额。
	for i := 0; i < 100; i++ {
		release, ok := l.acquire(context.Background(), 512)
		if !ok {
			t.Fatalf("small request must never be gated")
		}
		release()
	}
	if len(l.sem) != 0 {
		t.Fatalf("small requests must not consume slots, have %d", len(l.sem))
	}
}

func TestUploadLimiterBoundsConcurrency(t *testing.T) {
	l := newUploadLimiter(1<<10, 2, 0, 0) // 并发上限 2,等不到立即失败

	r1, ok1 := l.acquire(context.Background(), 1<<20)
	r2, ok2 := l.acquire(context.Background(), 1<<20)
	if !ok1 || !ok2 {
		t.Fatalf("first two large requests should acquire slots")
	}

	// 第三个大请求拿不到名额(无等待)→ 应被拒。
	if _, ok3 := l.acquire(context.Background(), 1<<20); ok3 {
		t.Fatalf("third concurrent large request must be shed when slots are full")
	}

	// 释放一个后,又能拿到。
	r1()
	r3, ok3 := l.acquire(context.Background(), 1<<20)
	if !ok3 {
		t.Fatalf("slot should be available after release")
	}
	r2()
	r3()
}

func TestUploadLimiterWaitsThenSucceeds(t *testing.T) {
	l := newUploadLimiter(1<<10, 1, 0, 500*time.Millisecond)
	r1, ok1 := l.acquire(context.Background(), 1<<20)
	if !ok1 {
		t.Fatalf("first large request should acquire")
	}
	// 在等待窗口内释放,第二个应当等到名额。
	go func() {
		time.Sleep(50 * time.Millisecond)
		r1()
	}()
	r2, ok2 := l.acquire(context.Background(), 1<<20)
	if !ok2 {
		t.Fatalf("second request should acquire after slot is freed within timeout")
	}
	r2()
}

func TestUploadLimiterRespectsContextCancel(t *testing.T) {
	l := newUploadLimiter(1<<10, 1, 0, 10*time.Second)
	r1, _ := l.acquire(context.Background(), 1<<20)
	defer r1()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if _, ok := l.acquire(ctx, 1<<20); ok {
		t.Fatalf("acquire should fail when context is cancelled while waiting")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("cancel should unblock acquire promptly, not wait full timeout")
	}
}

func TestUploadLimiterGatesAllUnknownLengthRequests(t *testing.T) {
	// 并发上限 1。Content-Length 未知(<0)一律按大上传门控,不论 content-type——
	// 无 Content-Length 无法预判体积,保守限流堵住分块上传绕过闸门的洞。
	l := newUploadLimiter(1<<10, 1, 0, 0)

	r1, ok1 := l.acquire(context.Background(), -1)
	if !ok1 {
		t.Fatalf("first unknown-length request should acquire a slot")
	}
	// 名额已满,第二个未知长度请求应被门控拒绝。
	if _, ok2 := l.acquire(context.Background(), -1); ok2 {
		t.Fatalf("second unknown-length request must be gated when slot full")
	}
	r1()
	// 释放后可再获取。
	if r3, ok3 := l.acquire(context.Background(), -1); !ok3 {
		t.Fatalf("slot should free up after release")
	} else {
		r3()
	}

	// 已知长度的小请求仍不门控、不占名额。
	if rel, ok := l.acquire(context.Background(), 512); !ok {
		t.Fatalf("known small request must not be gated")
	} else {
		rel()
	}
	// 空 body(Content-Length==0)不门控。
	if rel, ok := l.acquire(context.Background(), 0); !ok {
		t.Fatalf("empty body must not be gated")
	} else {
		rel()
	}
}

func TestUploadLimiterHardCap(t *testing.T) {
	l := newUploadLimiter(1<<10, 0, 1<<20, 0) // 无并发门控,硬上限 1MiB
	if !l.tooLarge(2 << 20) {
		t.Fatalf("2MiB content-length should exceed 1MiB hard cap")
	}
	if l.tooLarge(512 << 10) {
		t.Fatalf("512KiB should be under hard cap")
	}
	if l.tooLarge(-1) {
		t.Fatalf("unknown content-length must not be pre-rejected (LimitReader handles it)")
	}
}

func TestUploadLimiterDisabledByDefaultConcurrency(t *testing.T) {
	l := newUploadLimiter(1<<10, 0, 0, 0) // 并发上限 0 => 关闭门控
	if l.sem != nil {
		t.Fatalf("maxConcurrency<=0 must disable concurrency gating")
	}
	// 任意大请求都放行。
	if _, ok := l.acquire(context.Background(), 1<<30); !ok {
		t.Fatalf("gating disabled => always admit")
	}
}
