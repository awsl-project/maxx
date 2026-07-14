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

func TestApplyCodexHeadersUserAgentPreservesBlankOrMissing(t *testing.T) {
	upstreamReq, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	clientReq, _ := http.NewRequest("POST", "http://localhost/responses", nil)
	clientReq.Header.Set("User-Agent", "   ")

	applyCodexHeaders(upstreamReq, clientReq, "token-1")
	if got := upstreamReq.Header.Get("User-Agent"); got != "   " {
		t.Fatalf("expected blank client User-Agent passthrough, got %q", got)
	}

	upstreamReq2, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	applyCodexHeaders(upstreamReq2, nil, "token-1")
	if got := upstreamReq2.Header.Get("User-Agent"); got != "" {
		t.Fatalf("expected missing client User-Agent to remain empty, got %q", got)
	}
}

func TestApplyCodexHeadersPreservesVersionOnlyWhenProvided(t *testing.T) {
	clientReq, _ := http.NewRequest("POST", "http://localhost/responses", nil)
	clientReq.Header.Set("Version", "0.144.1")
	upstreamReq, _ := http.NewRequest("POST", "https://example.com/responses", nil)

	applyCodexHeaders(upstreamReq, clientReq, "token-1")
	if got := upstreamReq.Header.Get("Version"); got != "0.144.1" {
		t.Fatalf("expected client Version passthrough, got %q", got)
	}

	upstreamReq2, _ := http.NewRequest("POST", "https://example.com/responses", nil)
	applyCodexHeaders(upstreamReq2, nil, "token-1")
	if _, ok := upstreamReq2.Header["Version"]; ok {
		t.Fatalf("expected missing client Version to remain absent, got %q", upstreamReq2.Header.Get("Version"))
	}
}
