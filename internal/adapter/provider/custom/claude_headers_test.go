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

func TestApplyBedrockCompatHeadersStripsClaudeIdentity(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://relay.example.com/v1/messages", nil)
	// Pre-populate every Claude Code identifying header to verify they all get deleted.
	req.Header.Set("Anthropic-Beta", "claude-code-20250219,interleaved-thinking-2025-05-14")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")
	req.Header.Set("X-App", "cli")
	req.Header.Set("X-Stainless-Helper-Method", "stream")
	req.Header.Set("X-Stainless-Retry-Count", "0")
	req.Header.Set("X-Stainless-Runtime-Version", "v24.3.0")
	req.Header.Set("X-Stainless-Package-Version", "0.55.1")
	req.Header.Set("X-Stainless-Runtime", "node")
	req.Header.Set("X-Stainless-Lang", "js")
	req.Header.Set("X-Stainless-Arch", "arm64")
	req.Header.Set("X-Stainless-Os", "MacOS")
	req.Header.Set("X-Stainless-Timeout", "60")
	req.Header.Set("x-api-key", "leaked-key")

	applyBedrockCompatHeaders(req, nil, "sk-test", true)

	for _, h := range claudeIdentityHeaders {
		if v := req.Header.Get(h); v != "" {
			t.Errorf("expected %s to be stripped, got %q", h, v)
		}
	}
	if v := req.Header.Get("x-api-key"); v != "" {
		t.Errorf("expected x-api-key to be stripped, got %q", v)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("expected Authorization Bearer sk-test, got %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", got)
	}
	if got := req.Header.Get("User-Agent"); got != defaultBedrockCompatUserAgent {
		t.Errorf("expected User-Agent %q, got %q", defaultBedrockCompatUserAgent, got)
	}
	if !strings.Contains(req.Header.Get("User-Agent"), "aws-sdk-go-v2") {
		t.Errorf("User-Agent should look like AWS SDK, got %q", req.Header.Get("User-Agent"))
	}
}

func TestApplyBedrockCompatHeadersAccept(t *testing.T) {
	// Streaming must use text/event-stream so CustomAdapter.handleStreamResponse
	// (an SSE line parser) can decode the upstream response. The relay station
	// translates AWS Bedrock's binary event frames back to SSE before forwarding,
	// so asking for application/vnd.amazon.eventstream here would only break the
	// downstream parser.
	req, _ := http.NewRequest("POST", "https://relay.example.com/v1/messages", nil)
	applyBedrockCompatHeaders(req, nil, "sk-test", true)
	if got := req.Header.Get("Accept"); got != "text/event-stream" {
		t.Errorf("streaming Accept = %q, want text/event-stream", got)
	}

	// Non-streaming should yield application/json.
	req2, _ := http.NewRequest("POST", "https://relay.example.com/v1/messages", nil)
	applyBedrockCompatHeaders(req2, nil, "sk-test", false)
	if got := req2.Header.Get("Accept"); got != "application/json" {
		t.Errorf("non-streaming Accept = %q, want application/json", got)
	}
}

func TestSetClaudeAuthForURLPicksXAPIKeyForAnthropic(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer stale")
	setClaudeAuthForURL(req, "sk-test", true)
	if got := req.Header.Get("x-api-key"); got != "sk-test" {
		t.Errorf("x-api-key = %q, want sk-test", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be cleared, got %q", got)
	}
}

func TestSetClaudeAuthForURLPicksBearerForRelay(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://relay.example.com/v1/messages", nil)
	req.Header.Set("x-api-key", "stale-key")
	setClaudeAuthForURL(req, "sk-test", true)
	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", got)
	}
	if got := req.Header.Get("x-api-key"); got != "" {
		t.Errorf("x-api-key should be cleared on non-anthropic, got %q", got)
	}
}

func TestSetClaudeAuthForURLAnthropicWithoutUseAPIKeyFallsBackToBearer(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	setClaudeAuthForURL(req, "sk-test", false)
	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test (useAPIKey=false should fall back)", got)
	}
}

func TestSetClaudeAuthForURLEmptyAPIKeyNoOp(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://relay.example.com/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer untouched")
	setClaudeAuthForURL(req, "", true)
	if got := req.Header.Get("Authorization"); got != "Bearer untouched" {
		t.Errorf("Authorization should be untouched when apiKey empty, got %q", got)
	}
}

func TestSetClaudeAuthForURLClearsStaleSourceCredentialsOnConversion(t *testing.T) {
	// Simulates an OpenAI- and Gemini-origin request that just got format-converted
	// to Claude. Both source credentials must be wiped so the upstream sees only
	// the provider key.
	req, _ := http.NewRequest("POST", "https://relay.example.com/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer source-openai-key")
	req.Header.Set("x-goog-api-key", "source-gemini-key")
	req.Header.Set("x-api-key", "source-claude-key")

	setClaudeAuthForURL(req, "sk-provider", true)

	if got := req.Header.Get("Authorization"); got != "Bearer sk-provider" {
		t.Errorf("Authorization = %q, want Bearer sk-provider (relay → Bearer)", got)
	}
	if got := req.Header.Get("x-api-key"); got != "" {
		t.Errorf("stale x-api-key should be cleared, got %q", got)
	}
	if got := req.Header.Get("x-goog-api-key"); got != "" {
		t.Errorf("stale x-goog-api-key should be cleared, got %q", got)
	}
}

func TestSetClaudeAuthForURLAnthropicClearsBothOpposites(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer stale")
	req.Header.Set("x-goog-api-key", "stale-google")

	setClaudeAuthForURL(req, "sk-test", true)

	if got := req.Header.Get("x-api-key"); got != "sk-test" {
		t.Errorf("x-api-key = %q, want sk-test", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("stale Authorization should be cleared on anthropic.com, got %q", got)
	}
	if got := req.Header.Get("x-goog-api-key"); got != "" {
		t.Errorf("stale x-goog-api-key should be cleared on anthropic.com, got %q", got)
	}
}

func TestMergeBetaListDedupesAndPreservesOrder(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		extra    []string
		want     string
	}{
		{
			name:     "empty existing + extras",
			existing: "",
			extra:    []string{"a", "b"},
			want:     "a,b",
		},
		{
			name:     "existing then extras (no overlap)",
			existing: "a,b",
			extra:    []string{"c", "d"},
			want:     "a,b,c,d",
		},
		{
			name:     "dedupe extras already in existing",
			existing: "a,b,c",
			extra:    []string{"b", "d", "a"},
			want:     "a,b,c,d",
		},
		{
			name:     "trim whitespace and drop empty entries",
			existing: " a , , b ",
			extra:    []string{"  ", "c", "a"},
			want:     "a,b,c",
		},
		{
			name:     "no extras",
			existing: "a,b",
			extra:    nil,
			want:     "a,b",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mergeBetaList(tc.existing, tc.extra); got != tc.want {
				t.Errorf("mergeBetaList(%q, %v) = %q, want %q", tc.existing, tc.extra, got, tc.want)
			}
		})
	}
}

func TestApplyBedrockCompatHeadersWithoutAPIKeyClearsPreexistingAuth(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://relay.example.com/v1/messages", nil)
	// Pre-populated auth headers must be cleared even when apiKey is empty —
	// otherwise caller-side state could leak through to the upstream.
	req.Header.Set("x-api-key", "preexisting-key")
	req.Header.Set("Authorization", "Bearer preexisting-bearer")

	applyBedrockCompatHeaders(req, nil, "", true)

	if got := req.Header.Get("x-api-key"); got != "" {
		t.Errorf("expected x-api-key to be cleared, got %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("expected Authorization to be cleared, got %q", got)
	}
}

// TestApplyBedrockCompatHeadersFromClaudeBaseline simulates the realistic flow:
// adapter.go first sets the full Claude Code header set on upstreamReq, and
// then the bedrock disguise wrapper has to scrub all of them back out.
func TestApplyBedrockCompatHeadersFromClaudeBaseline(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://relay.example.com/v1/messages", nil)
	clientReq, _ := http.NewRequest("POST", "https://example.com", nil)
	// Run the regular Claude Code header set first to get a realistic starting state.
	applyClaudeHeaders(req, clientReq, "sk-old", true, []string{"some-beta"}, true)

	// Now apply bedrock disguise — every Claude Code fingerprint must be gone.
	applyBedrockCompatHeaders(req, nil, "sk-new", true)

	for _, h := range claudeIdentityHeaders {
		if got := req.Header.Get(h); got != "" {
			t.Errorf("expected %s to be stripped, still got %q", h, got)
		}
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-new" {
		t.Errorf("Authorization should be the new key, got %q", got)
	}
	if !strings.HasPrefix(req.Header.Get("User-Agent"), "aws-sdk-go-v2") {
		t.Errorf("User-Agent should be replaced with AWS SDK string, got %q", req.Header.Get("User-Agent"))
	}
}
