package handler

import (
	"encoding/json"
	"io"
	"log"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestWebSocketHub_BroadcastProxyRequest_SendsSnapshot(t *testing.T) {
	hub := &WebSocketHub{
		broadcast: make(chan WSMessage, 1),
	}

	req := &domain.ProxyRequest{
		ID:        1,
		RequestID: "req_1",
		Status:    "IN_PROGRESS",
	}

	hub.BroadcastProxyRequest(req)

	// 如果 Broadcast 发送的是同一个指针，那么这里对原对象的修改会“污染”队列中的消息。
	req.Status = "COMPLETED"

	msg := <-hub.broadcast
	if msg.Type != "proxy_request_update" {
		t.Fatalf("unexpected message type: %s", msg.Type)
	}

	switch v := msg.Data.(type) {
	case *domain.ProxyRequest:
		if v == req {
			t.Fatalf("expected snapshot (different pointer), got original pointer")
		}
		if v.Status != "IN_PROGRESS" {
			t.Fatalf("expected snapshot status IN_PROGRESS, got %s", v.Status)
		}
	case domain.ProxyRequest:
		if v.Status != "IN_PROGRESS" {
			t.Fatalf("expected snapshot status IN_PROGRESS, got %s", v.Status)
		}
	default:
		t.Fatalf("unexpected data type: %T", msg.Data)
	}
}

func TestWebSocketHub_BroadcastProxyUpstreamAttempt_SendsSnapshot(t *testing.T) {
	hub := &WebSocketHub{
		broadcast: make(chan WSMessage, 1),
	}

	attempt := &domain.ProxyUpstreamAttempt{
		ID:             2,
		ProxyRequestID: 1,
		Status:         "IN_PROGRESS",
	}

	hub.BroadcastProxyUpstreamAttempt(attempt)
	attempt.Status = "COMPLETED"

	msg := <-hub.broadcast
	if msg.Type != "proxy_upstream_attempt_update" {
		t.Fatalf("unexpected message type: %s", msg.Type)
	}

	switch v := msg.Data.(type) {
	case *domain.ProxyUpstreamAttempt:
		if v == attempt {
			t.Fatalf("expected snapshot (different pointer), got original pointer")
		}
		if v.Status != "IN_PROGRESS" {
			t.Fatalf("expected snapshot status IN_PROGRESS, got %s", v.Status)
		}
	case domain.ProxyUpstreamAttempt:
		if v.Status != "IN_PROGRESS" {
			t.Fatalf("expected snapshot status IN_PROGRESS, got %s", v.Status)
		}
	default:
		t.Fatalf("unexpected data type: %T", msg.Data)
	}
}

func TestWebSocketHub_BroadcastDrop_IncrementsCounter(t *testing.T) {
	hub := &WebSocketHub{
		broadcast: make(chan WSMessage, 1),
	}
	hub.broadcast <- WSMessage{Type: "dummy", Data: nil}

	before := hub.broadcastDroppedTotal.Load()

	req := &domain.ProxyRequest{
		ID:        1,
		RequestID: "req_1",
		Status:    "IN_PROGRESS",
	}
	hub.BroadcastProxyRequest(req)

	after := hub.broadcastDroppedTotal.Load()
	if after != before+1 {
		t.Fatalf("expected drop counter to increment from %d to %d, got %d", before, before+1, after)
	}
}

func TestWebSocketLogWriter_NoDeadlockOnFullChannel(t *testing.T) {
	// Create hub WITHOUT starting run() goroutine, so channel stays full
	hub := &WebSocketHub{
		broadcast: make(chan WSMessage, 100),
	}

	// Fill broadcast channel completely
	for i := 0; i < 100; i++ {
		hub.broadcast <- WSMessage{Type: "fill", Data: i}
	}

	// Create WebSocketLogWriter pointing to this hub
	writer := NewWebSocketLogWriter(hub, io.Discard, "")

	// Redirect log output through WebSocketLogWriter
	oldOutput := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(writer)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldOutput)
		log.SetFlags(oldFlags)
	}()

	// log.Printf path: holds log mutex → WebSocketLogWriter.Write
	//   → BroadcastLog → tryEnqueueBroadcast → channel full → default branch
	// Before fix: default branch called log.Printf → re-acquire log mutex → DEADLOCK
	// After fix:  default branch only increments counter → no deadlock
	done := make(chan struct{})
	go func() {
		log.Printf("this must not deadlock")
		close(done)
	}()

	select {
	case <-done:
		// No deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("Deadlock: log.Printf hung because tryEnqueueBroadcast called log.Printf while log mutex was held")
	}
}

func TestWebSocketHub_BroadcastMessage_SendsSnapshot(t *testing.T) {
	hub := &WebSocketHub{
		broadcast: make(chan WSMessage, 1),
	}

	type payload struct {
		A int `json:"a"`
	}

	p := &payload{A: 1}
	hub.BroadcastMessage("custom_event", p)

	// 如果 BroadcastMessage 直接把指针放进队列，这里修改会污染后续消费者看到的数据。
	p.A = 2

	msg := <-hub.broadcast
	if msg.Type != "custom_event" {
		t.Fatalf("unexpected message type: %s", msg.Type)
	}

	raw, ok := msg.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("expected Data to be json.RawMessage snapshot, got %T", msg.Data)
	}

	var got payload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to unmarshal snapshot: %v", err)
	}
	if got.A != 1 {
		t.Fatalf("expected snapshot A=1, got %d", got.A)
	}
}
