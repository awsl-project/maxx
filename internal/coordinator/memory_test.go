package coordinator

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestMemoryKVRoundtrip(t *testing.T) {
	c := NewMemory("inst-1")
	defer c.Close()
	ctx := context.Background()

	if _, err := c.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing key, got %v", err)
	}

	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := c.Get(ctx, "k")
	if err != nil || string(got) != "v" {
		t.Fatalf("Get after Set: %q err=%v", got, err)
	}

	if err := c.Del(ctx, "k"); err != nil {
		t.Fatalf("Del: %v", err)
	}
	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Del, got %v", err)
	}
}

func TestMemoryTTLExpiry(t *testing.T) {
	c := NewMemory("inst-1")
	defer c.Close()
	ctx := context.Background()

	if err := c.Set(ctx, "k", []byte("v"), 30*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected expiry, got err=%v", err)
	}
}

func TestMemoryPubSub(t *testing.T) {
	c := NewMemory("inst-1")
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := c.Subscribe(ctx, "evt")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var got Message
	go func() {
		defer wg.Done()
		select {
		case got = <-ch:
		case <-time.After(time.Second):
			t.Errorf("timeout waiting for message")
		}
	}()

	if err := c.Publish(ctx, "evt", []byte("hello")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	wg.Wait()

	if got.Sender != "inst-1" || string(got.Payload) != "hello" {
		t.Fatalf("unexpected message: %+v", got)
	}
}

func TestMemorySubscribeStopsOnContext(t *testing.T) {
	c := NewMemory("inst-1")
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := c.Subscribe(ctx, "evt")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected channel closed after ctx cancel")
		}
	case <-time.After(time.Second):
		t.Fatalf("channel not closed after ctx cancel")
	}
}

func TestMemoryInstanceHeartbeat(t *testing.T) {
	c := NewMemory("inst-1")
	defer c.Close()
	ctx := context.Background()

	alive, err := c.ListAliveInstances(ctx)
	if err != nil || len(alive) != 0 {
		t.Fatalf("expected empty alive list, got %v err=%v", alive, err)
	}

	if err := c.RegisterInstance(ctx, time.Second); err != nil {
		t.Fatalf("Register: %v", err)
	}
	alive, _ = c.ListAliveInstances(ctx)
	if len(alive) != 1 || alive[0] != "inst-1" {
		t.Fatalf("expected [inst-1], got %v", alive)
	}

	if err := c.UnregisterInstance(ctx); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	alive, _ = c.ListAliveInstances(ctx)
	if len(alive) != 0 {
		t.Fatalf("expected empty after Unregister, got %v", alive)
	}
}

func TestMemoryInstanceExpiry(t *testing.T) {
	c := NewMemory("inst-1")
	defer c.Close()
	ctx := context.Background()

	_ = c.RegisterInstance(ctx, 20*time.Millisecond)
	time.Sleep(40 * time.Millisecond)
	alive, _ := c.ListAliveInstances(ctx)
	if len(alive) != 0 {
		t.Fatalf("expected expired, got %v", alive)
	}
}

func TestMemoryMultipleSubscribers(t *testing.T) {
	c := NewMemory("inst-1")
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const n = 3
	chans := make([]<-chan Message, n)
	for i := 0; i < n; i++ {
		ch, err := c.Subscribe(ctx, "evt")
		if err != nil {
			t.Fatalf("Subscribe %d: %v", i, err)
		}
		chans[i] = ch
	}

	_ = c.Publish(ctx, "evt", []byte("x"))

	var got []string
	for i, ch := range chans {
		select {
		case m := <-ch:
			got = append(got, string(m.Payload))
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timeout", i)
		}
	}
	sort.Strings(got)
	if len(got) != n {
		t.Fatalf("expected %d deliveries, got %d", n, len(got))
	}
}
