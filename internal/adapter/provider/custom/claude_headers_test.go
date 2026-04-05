package custom

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestApplyClaudeHeadersAccept(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	clientReq, _ := http.NewRequest("POST", "https://example.com", nil)

	applyClaudeHeaders(req, clientReq, "sk-test", true, nil, false)
	if req.Header.Get("Accept") != "application/json" {
		t.Errorf("expected Accept application/json, got %s", req.Header.Get("Accept"))
	}

	req2, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	applyClaudeHeaders(req2, clientReq, "sk-test", true, nil, true)
	if req2.Header.Get("Accept") != "text/event-stream" {
		t.Errorf("expected Accept text/event-stream, got %s", req2.Header.Get("Accept"))
	}
}

func TestApplyClaudeHeadersAuthSelection(t *testing.T) {
	anthropicReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	applyClaudeHeaders(anthropicReq, nil, "sk-test", true, nil, true)
	if anthropicReq.Header.Get("x-api-key") != "sk-test" {
		t.Errorf("expected x-api-key set for anthropic base")
	}
	if strings.Contains(anthropicReq.Header.Get("Authorization"), "Bearer") {
		t.Errorf("expected Authorization not set for anthropic base")
	}

	customReq, _ := http.NewRequest("POST", "https://proxy.example.com/v1/messages", nil)
	applyClaudeHeaders(customReq, nil, "sk-test", true, nil, true)
	if customReq.Header.Get("Authorization") != "Bearer sk-test" {
		t.Errorf("expected Authorization Bearer for non-anthropic base")
	}
	if customReq.Header.Get("x-api-key") != "" {
		t.Errorf("expected x-api-key not set for non-anthropic base")
	}

	oauthReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	applyClaudeHeaders(oauthReq, nil, "sk-ant-oat-123", false, nil, true)
	if oauthReq.Header.Get("Authorization") != "Bearer sk-ant-oat-123" {
		t.Errorf("expected Authorization Bearer for OAuth token on anthropic base")
	}
	if oauthReq.Header.Get("x-api-key") != "" {
		t.Errorf("expected x-api-key not set for OAuth token on anthropic base")
	}

	bearerReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	applyClaudeHeaders(bearerReq, nil, "sk-test", false, nil, true)
	if bearerReq.Header.Get("Authorization") != "Bearer sk-test" {
		t.Errorf("expected Authorization Bearer when useAPIKey=false")
	}
	if bearerReq.Header.Get("x-api-key") != "" {
		t.Errorf("expected x-api-key not set when useAPIKey=false")
	}
}

func TestApplyClaudeHeadersBetas(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	clientReq, _ := http.NewRequest("POST", "https://example.com", nil)
	clientReq.Header.Set("Anthropic-Beta", "custom-beta")

	applyClaudeHeaders(req, clientReq, "sk-test", true, []string{"extra-beta"}, true)
	beta := req.Header.Get("Anthropic-Beta")
	if !strings.Contains(beta, "custom-beta") {
		t.Errorf("expected custom-beta to be preserved, got %s", beta)
	}
	if !strings.Contains(beta, "oauth-2025-04-20") {
		t.Errorf("expected oauth beta to be appended, got %s", beta)
	}
	if !strings.Contains(beta, "prompt-caching-2024-07-31") {
		t.Errorf("expected prompt-caching beta to be present, got %s", beta)
	}
	if !strings.Contains(beta, "extra-beta") {
		t.Errorf("expected extra beta to be merged, got %s", beta)
	}
}

func TestApplyClaudeHeadersDefaults(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	applyClaudeHeaders(req, nil, "sk-test", true, nil, true)

	if req.Header.Get("Anthropic-Version") == "" {
		t.Error("Anthropic-Version should be set")
	}
	if req.Header.Get("User-Agent") == "" {
		t.Error("User-Agent should be set")
	}
	if req.Header.Get("X-Stainless-Runtime") == "" {
		t.Error("X-Stainless-Runtime should be set")
	}
}

func TestApplyClaudeHeadersUserAgentPassthroughWhenProvided(t *testing.T) {
	cliReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	cliClientReq, _ := http.NewRequest("POST", "https://example.com", nil)
	cliClientReq.Header.Set("User-Agent", "claude-cli/2.1.23 (external, cli)")

	applyClaudeHeaders(cliReq, cliClientReq, "sk-test", true, nil, true)
	if got := cliReq.Header.Get("User-Agent"); got != "claude-cli/2.1.23 (external, cli)" {
		t.Fatalf("expected CLI User-Agent passthrough, got %q", got)
	}

	nonCLIReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	nonCLIClientReq, _ := http.NewRequest("POST", "https://example.com", nil)
	nonCLIClientReq.Header.Set("User-Agent", "Mozilla/5.0")

	applyClaudeHeaders(nonCLIReq, nonCLIClientReq, "sk-test", true, nil, true)
	if got := nonCLIReq.Header.Get("User-Agent"); got != "Mozilla/5.0" {
		t.Fatalf("expected non-CLI User-Agent passthrough, got %q", got)
	}

	nonOfficialReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	nonOfficialClientReq, _ := http.NewRequest("POST", "https://example.com", nil)
	nonOfficialClientReq.Header.Set("User-Agent", "claude-cli/dev")

	applyClaudeHeaders(nonOfficialReq, nonOfficialClientReq, "sk-test", true, nil, true)
	if got := nonOfficialReq.Header.Get("User-Agent"); got != "claude-cli/dev" {
		t.Fatalf("expected arbitrary User-Agent passthrough, got %q", got)
	}
}

func TestApplyClaudeHeadersUserAgentFallsBackWhenMissing(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	clientReq, _ := http.NewRequest("POST", "https://example.com", nil)
	clientReq.Header.Set("User-Agent", "   ")

	applyClaudeHeaders(req, clientReq, "sk-test", true, nil, true)
	if got := req.Header.Get("User-Agent"); got != defaultClaudeUserAgent {
		t.Fatalf("expected default User-Agent when client UA is blank, got %q", got)
	}

	req2, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	applyClaudeHeaders(req2, nil, "sk-test", true, nil, true)
	if got := req2.Header.Get("User-Agent"); got != defaultClaudeUserAgent {
		t.Fatalf("expected default User-Agent when client request is nil, got %q", got)
	}
}

func TestCloakingBuildsSub2apiCompatibleClaudeShape(t *testing.T) {
	clientReq, _ := http.NewRequest("POST", "https://example.com/v1/messages", nil)
	clientReq.Header.Set("User-Agent", "curl/8.0.0")

	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`)
	processedBody, extraBetas := processClaudeRequestBody(body, clientReq.Header.Get("User-Agent"), nil)

	upstreamReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	applyClaudeHeaders(upstreamReq, clientReq, "sk-test", true, extraBetas, true)

	if got := upstreamReq.Header.Get("User-Agent"); got != "curl/8.0.0" {
		t.Fatalf("expected original User-Agent passthrough, got %q", got)
	}

	for _, key := range []string{"X-App", "Anthropic-Beta", "Anthropic-Version"} {
		if strings.TrimSpace(upstreamReq.Header.Get(key)) == "" {
			t.Fatalf("expected %s to be set", key)
		}
	}

	userID := gjson.GetBytes(processedBody, "metadata.user_id").String()
	userIDPattern := regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !userIDPattern.MatchString(userID) {
		t.Fatalf("expected sub2api-compatible metadata.user_id, got %q", userID)
	}

	systemText := gjson.GetBytes(processedBody, "system.0.text").String()
	if !strings.Contains(systemText, "Claude Code, Anthropic's official CLI for Claude") {
		t.Fatalf("expected cloaked system prompt, got %q", systemText)
	}
}
