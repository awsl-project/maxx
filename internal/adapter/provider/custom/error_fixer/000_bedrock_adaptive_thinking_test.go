package error_fixer

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func TestBedrockAdaptiveThinkingFixer_MatchResponse(t *testing.T) {
	f := &bedrockAdaptiveThinkingFixer{}

	body := []byte(`{"error":{"message":"InvokeModelWithResponseStream: operation error Bedrock Runtime: InvokeModelWithResponseStream, https response error StatusCode: 400, ValidationException: \"thinking.type.enabled\" is not supported for this model. Use \"thinking.type.adaptive\" and \"output_config.effort\" to control thinking behavior."}}`)

	if !f.MatchResponse(&http.Response{StatusCode: http.StatusBadRequest}, body, domain.ClientTypeClaude) {
		t.Fatal("expected adaptive-thinking Bedrock schema error to match")
	}
	if f.MatchResponse(&http.Response{StatusCode: http.StatusBadRequest}, body, domain.ClientTypeOpenAI) {
		t.Fatal("OpenAI client type should not match")
	}
}

func TestBedrockAdaptiveThinkingFixer_FixRequest(t *testing.T) {
	f := &bedrockAdaptiveThinkingFixer{}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)

	_, result := f.FixRequest(req, []byte(`{
		"thinking": {"type":"enabled","budget_tokens":32000},
		"temperature": 0.2,
		"top_p": 0.9,
		"top_k": 10,
		"max_tokens": 64000
	}`))

	if got := gjson.GetBytes(result, "thinking.type").String(); got != "adaptive" {
		t.Fatalf("thinking.type = %q, want adaptive", got)
	}
	if gjson.GetBytes(result, "thinking.budget_tokens").Exists() {
		t.Fatal("thinking.budget_tokens should be removed")
	}
	if got := gjson.GetBytes(result, "output_config.effort").String(); got != "high" {
		t.Fatalf("output_config.effort = %q, want high", got)
	}
	for _, field := range []string{"temperature", "top_p", "top_k"} {
		if gjson.GetBytes(result, field).Exists() {
			t.Fatalf("%s should be stripped for adaptive-thinking retry", field)
		}
	}
}
