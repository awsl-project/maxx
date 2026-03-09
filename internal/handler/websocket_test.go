package handler

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadLogRotationConfig_UsesDefaultsOnInvalidEnv(t *testing.T) {
	t.Setenv(logMaxSizeEnv, "invalid")
	t.Setenv(logMaxAgeEnv, "-5")
	t.Setenv(logMaxBackupsEnv, "0")
	t.Setenv(logCompressEnv, "definitely-not-bool")

	cfg := loadLogRotationConfig()

	if cfg.MaxSizeMB != defaultLogMaxSizeMB {
		t.Fatalf("expected default max size %d, got %d", defaultLogMaxSizeMB, cfg.MaxSizeMB)
	}
	if cfg.MaxAgeDays != defaultLogMaxAgeDays {
		t.Fatalf("expected default max age %d, got %d", defaultLogMaxAgeDays, cfg.MaxAgeDays)
	}
	if cfg.MaxBackups != defaultLogMaxBackups {
		t.Fatalf("expected default max backups %d, got %d", defaultLogMaxBackups, cfg.MaxBackups)
	}
	if cfg.Compress != defaultLogCompress {
		t.Fatalf("expected default compress %t, got %t", defaultLogCompress, cfg.Compress)
	}
}

func TestWebSocketLogWriter_RotatesAndCompressesLogFile(t *testing.T) {
	t.Setenv(logMaxSizeEnv, "1")
	t.Setenv(logMaxBackupsEnv, "1")
	t.Setenv(logMaxAgeEnv, "1")
	t.Setenv(logCompressEnv, "true")

	dir := t.TempDir()
	logPath := filepath.Join(dir, "maxx.log")
	hub := &WebSocketHub{
		broadcast: make(chan WSMessage, 16),
	}
	writer := NewWebSocketLogWriter(hub, io.Discard, logPath)

	chunk := []byte(strings.Repeat("x", 256*1024))
	for i := 0; i < 10; i++ {
		if _, err := writer.Write(chunk); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}
	if err := writer.logFile.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	matches, err := waitForGlob(filepath.Join(dir, "maxx-*.log.gz"), 3*time.Second)
	if err != nil {
		t.Fatalf("expected compressed rotated log: %v", err)
	}
	if len(matches) > 1 {
		t.Fatalf("expected at most one rotated backup, got %d (%v)", len(matches), matches)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("expected active log file to exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected active log file to contain recent log data")
	}
}

func waitForGlob(pattern string, timeout time.Duration) ([]string, error) {
	deadline := time.Now().Add(timeout)
	for {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		if len(matches) > 0 {
			return matches, nil
		}
		if time.Now().After(deadline) {
			return nil, os.ErrNotExist
		}
		time.Sleep(50 * time.Millisecond)
	}
}
