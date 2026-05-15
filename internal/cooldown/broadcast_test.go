package cooldown

import (
	"context"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
)

// applyRemoteEvent 应用一条 set 事件,本地内存出现对应 key。
func TestApplyRemoteSet(t *testing.T) {
	m := NewManager()
	until := time.Now().Add(30 * time.Second)
	m.applyRemoteEvent(cooldownEvent{
		Op:         opSet,
		ProviderID: 7,
		ClientType: "claude",
		Model:      "opus",
		UntilUnix:  until.Unix(),
		Reason:     string(ReasonUnknown),
	})

	if !m.IsInCooldown(7, "claude", "opus") {
		t.Fatal("expected provider in cooldown after remote set")
	}
}

func TestApplyRemoteClear(t *testing.T) {
	m := NewManager()
	until := time.Now().Add(30 * time.Second)
	m.applyRemoteEvent(cooldownEvent{
		Op: opSet, ProviderID: 1, ClientType: "c", Model: "m",
		UntilUnix: until.Unix(), Reason: string(ReasonUnknown),
	})
	m.applyRemoteEvent(cooldownEvent{
		Op: opClear, ProviderID: 1, ClientType: "c", Model: "m",
	})
	if m.IsInCooldown(1, "c", "m") {
		t.Fatal("expected cleared after remote clear")
	}
}

func TestApplyRemoteClearAll(t *testing.T) {
	m := NewManager()
	until := time.Now().Add(30 * time.Second)
	for _, model := range []string{"a", "b", "c"} {
		m.applyRemoteEvent(cooldownEvent{
			Op: opSet, ProviderID: 1, ClientType: "claude", Model: model,
			UntilUnix: until.Unix(), Reason: string(ReasonUnknown),
		})
	}
	// 另一个 provider 不应受影响
	m.applyRemoteEvent(cooldownEvent{
		Op: opSet, ProviderID: 2, ClientType: "claude", Model: "z",
		UntilUnix: until.Unix(), Reason: string(ReasonUnknown),
	})

	m.applyRemoteEvent(cooldownEvent{Op: opClearAll, ProviderID: 1})

	for _, model := range []string{"a", "b", "c"} {
		if m.IsInCooldown(1, "claude", model) {
			t.Fatalf("provider 1 model %s should be cleared", model)
		}
	}
	if !m.IsInCooldown(2, "claude", "z") {
		t.Fatal("provider 2 should remain in cooldown")
	}
}

// SetCoordinator 之后,本地写操作发出广播。这里用单个 coordinator + 直接
// Subscribe 验证 publish 链路;sender 过滤逻辑在 SetCoordinator 内部的订阅
// goroutine 里,通过同 coord 无法验证。
func TestSetCooldownPublishesBroadcast(t *testing.T) {
	m := NewManager()
	c := coordinator.NewMemory("inst-X")
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rawCh, err := c.Subscribe(ctx, broadcastChannel)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	m.SetCoordinator(ctx, c)
	m.SetCooldownDuration(42, "claude", "opus", 30*time.Second)

	select {
	case msg := <-rawCh:
		if msg.Sender != "inst-X" {
			t.Fatalf("sender = %q, want inst-X", msg.Sender)
		}
	case <-time.After(time.Second):
		t.Fatal("did not observe broadcast for SetCooldown")
	}
}
