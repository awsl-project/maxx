package custom

import (
	"net/http"
	"testing"
)

func TestApplyCodexHeadersUserAgentPassthroughWhenProvided(t *testing.T) {
	upstreamReq, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	clientReq, _ := http.NewRequest("POST", "http://localhost/responses", nil)
	clientReq.Header.Set("User-Agent", "codex-cli/1.2.3")

	applyCodexHeaders(upstreamReq, clientReq, "token-1")
	if got := upstreamReq.Header.Get("User-Agent"); got != "codex-cli/1.2.3" {
		t.Fatalf("expected CLI User-Agent passthrough, got %q", got)
	}

	upstreamReq2, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	clientReq2, _ := http.NewRequest("POST", "http://localhost/responses", nil)
	clientReq2.Header.Set("User-Agent", "Mozilla/5.0")

	applyCodexHeaders(upstreamReq2, clientReq2, "token-1")
	if got := upstreamReq2.Header.Get("User-Agent"); got != "Mozilla/5.0" {
		t.Fatalf("expected non-CLI User-Agent passthrough, got %q", got)
	}
}

func TestApplyCodexHeadersUserAgentFallsBackWhenMissing(t *testing.T) {
	upstreamReq, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	clientReq, _ := http.NewRequest("POST", "http://localhost/responses", nil)
	clientReq.Header.Set("User-Agent", "   ")

	applyCodexHeaders(upstreamReq, clientReq, "token-1")
	if got := upstreamReq.Header.Get("User-Agent"); got != codexUserAgent {
		t.Fatalf("expected default User-Agent when client UA is blank, got %q", got)
	}

	upstreamReq2, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	applyCodexHeaders(upstreamReq2, nil, "token-1")
	if got := upstreamReq2.Header.Get("User-Agent"); got != codexUserAgent {
		t.Fatalf("expected default User-Agent when client request is nil, got %q", got)
	}
}
