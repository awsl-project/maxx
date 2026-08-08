package error_fixer

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func TestBedrockClassicThinkingFixer_MatchResponse(t *testing.T) {
	f := &bedrockClassicThinkingFixer{}

	body := []byte(`{"error":{"message":"InvokeModel: operation error Bedrock Runtime: InvokeModel, https response error StatusCode: 400, ValidationException: output_config.effort: Extra inputs are not permitted"}}`)

	if !f.MatchResponse(&http.Response{StatusCode: http.StatusBadRequest}, body, domain.ClientTypeClaude) {
		t.Fatal("expected adaptive rejection to match classic-thinking fallback")
	}
	if f.MatchResponse(&http.Response{StatusCode: http.StatusBadRequest}, body, domain.ClientTypeOpenAI) {
		t.Fatal("OpenAI client type should not match")
	}
}

func TestBedrockClassicThinkingFixer_FixRequest(t *testing.T) {
	f := &bedrockClassicThinkingFixer{}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)

	_, result := f.FixRequest(req, []byte(`{
		"thinking": {"type":"adaptive"},
		"output_config": {"effort":"medium"},
		"max_tokens": 200
	}`))

	if got := gjson.GetBytes(result, "thinking.type").String(); got != "enabled" {
		t.Fatalf("thinking.type = %q, want enabled", got)
	}
	if got := gjson.GetBytes(result, "thinking.budget_tokens").Int(); got != 8192 {
		t.Fatalf("thinking.budget_tokens = %d, want 8192", got)
	}
	if gjson.GetBytes(result, "output_config").Exists() {
		t.Fatal("output_config should be removed")
	}
	if got := gjson.GetBytes(result, "max_tokens").Int(); got != 8193 {
		t.Fatalf("max_tokens = %d, want 8193", got)
	}
}

func TestBedrockClassicThinkingFixer_FixRequestWithoutAdaptiveFallsBackToStrip(t *testing.T) {
	f := &bedrockClassicThinkingFixer{}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)

	_, result := f.FixRequest(req, []byte(`{
		"output_config": {"effort":"high"},
		"context_management": {"truncation":"auto"},
		"reasoning": {"budget_tokens":4096},
		"messages": [{"role":"user","content":"hi"}]
	}`))

	for _, field := range []string{"output_config", "context_management", "reasoning"} {
		if gjson.GetBytes(result, field).Exists() {
			t.Fatalf("%s should be stripped", field)
		}
	}
	if !gjson.GetBytes(result, "messages").Exists() {
		t.Fatal("messages should be preserved")
	}
}
