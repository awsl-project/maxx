package claude

import (
	"net/http"
	"testing"
)

func TestApplyClaudeHeadersPreservesProvidedUA(t *testing.T) {
	a := &ClaudeAdapter{}
	upstreamReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	clientReq, _ := http.NewRequest("POST", "http://localhost/v1/messages", nil)
	clientReq.Header.Set("User-Agent", "Mozilla/5.0 maxx-ua-regression")
	clientReq.Header.Set("X-Custom", "ok")
	clientReq.Header.Set("X-Forwarded-For", "1.2.3.4")

	a.applyClaudeHeaders(upstreamReq, clientReq, "sk-ant-oat-123", true, nil)

	if got := upstreamReq.Header.Get("User-Agent"); got != "Mozilla/5.0 maxx-ua-regression" {
		t.Fatalf("expected provided User-Agent passthrough, got %q", got)
	}
	if got := upstreamReq.Header.Get("X-Custom"); got != "ok" {
		t.Fatalf("expected X-Custom passthrough, got %q", got)
	}
	if got := upstreamReq.Header.Get("X-Forwarded-For"); got != "" {
		t.Fatalf("expected X-Forwarded-For filtered, got %q", got)
	}
}

func TestApplyClaudeHeadersUsesDefaultUAWhenMissing(t *testing.T) {
	a := &ClaudeAdapter{}
	upstreamReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	clientReq, _ := http.NewRequest("POST", "http://localhost/v1/messages", nil)
	clientReq.Header.Set("User-Agent", "   ")

	a.applyClaudeHeaders(upstreamReq, clientReq, "sk-ant-oat-123", true, nil)

	if got := upstreamReq.Header.Get("User-Agent"); got != ClaudeUserAgent {
		t.Fatalf("expected default Claude User-Agent when client UA is blank, got %q", got)
	}

	upstreamReq2, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	a.applyClaudeHeaders(upstreamReq2, nil, "sk-ant-oat-123", true, nil)
	if got := upstreamReq2.Header.Get("User-Agent"); got != ClaudeUserAgent {
		t.Fatalf("expected default Claude User-Agent when client request is nil, got %q", got)
	}
}
