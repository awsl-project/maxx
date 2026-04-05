package error_fixer

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func TestBedrockFixer_MatchResponse(t *testing.T) {
	f := &bedrockFixer{}

	tests := []struct {
		name       string
		resp       *http.Response
		body       string
		clientType domain.ClientType
		want       bool
	}{
		{
			name:       "Bedrock InvokeModel error",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"InvokeModel: operation error Bedrock Runtime: InvokeModel, https response error StatusCode: 400, ValidationException: output_config.effort: Extra inputs are not permitted"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "Bedrock InvokeModelWithResponseStream",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"InvokeModelWithResponseStream: operation error Bedrock Runtime: InvokeModelWithResponseStream, https response error StatusCode: 400"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "Bedrock Runtime in masked proxy error",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"***: Bedrock Runtime error"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "generic Extra inputs NOT from Bedrock — should NOT match",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"output_config: Extra inputs are not permitted"}}`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "generic ValidationException without Bedrock — should NOT match",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"ValidationException: bad input"}}`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "nil response",
			resp:       nil,
			body:       `Bedrock Runtime`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "wrong client type",
			resp:       &http.Response{StatusCode: 400},
			body:       `InvokeModel: Bedrock Runtime error`,
			clientType: domain.ClientTypeGemini,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.MatchResponse(tt.resp, []byte(tt.body), tt.clientType)
			if got != tt.want {
				t.Errorf("MatchResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBedrockFixer_FixRequest_StripsAll(t *testing.T) {
	f := &bedrockFixer{}

	input := []byte(`{
		"model": "claude-haiku-4-5-20251001",
		"max_tokens": 1024,
		"system": [{"type":"text","text":"hello","cache_control":{"type":"ephemeral","scope":"turn"}}],
		"output_config": {"effort": "high"},
		"context_management": {"truncation": "auto"},
		"reasoning": {"budget_tokens": 5000},
		"tools": [
			{"name":"bash","description":"run","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral","scope":"turn"},"input_schema":{"type":"object"}},
			{"name":"read","description":"read","custom":{"eager_input_streaming":true}}
		],
		"messages": [{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","scope":"turn"}}]}]
	}`)

	req, _ := http.NewRequest("POST", "https://example.com", nil)
	req.Header.Set("anthropic-beta", "prompt-caching-scope-2026-01-05,output-128k-2025-02-19")

	retReq, result := f.FixRequest(req, input)

	// cache_control.type preserved but scope stripped
	if !gjson.GetBytes(result, "system.0.cache_control").Exists() {
		t.Error("cache_control should be preserved in system")
	}
	if gjson.GetBytes(result, "system.0.cache_control.type").String() != "ephemeral" {
		t.Error("cache_control.type should be preserved")
	}
	if gjson.GetBytes(result, "system.0.cache_control.scope").Exists() {
		t.Error("cache_control.scope should be stripped from system")
	}
	if gjson.GetBytes(result, "tools.0.cache_control.scope").Exists() {
		t.Error("cache_control.scope should be stripped from tools")
	}
	if gjson.GetBytes(result, "messages.0.content.0.cache_control.scope").Exists() {
		t.Error("cache_control.scope should be stripped from messages")
	}
	if gjson.GetBytes(result, "messages.0.content.0.cache_control.type").String() != "ephemeral" {
		t.Error("cache_control.type should be preserved in messages")
	}
	if gjson.GetBytes(result, "output_config").Exists() {
		t.Error("output_config not stripped")
	}
	if gjson.GetBytes(result, "context_management").Exists() {
		t.Error("context_management not stripped")
	}
	if gjson.GetBytes(result, "reasoning").Exists() {
		t.Error("reasoning not stripped")
	}
	if gjson.GetBytes(result, "tools.0.custom").Exists() {
		t.Error("tools[0].custom not stripped")
	}
	if gjson.GetBytes(result, "tools.1.custom").Exists() {
		t.Error("tools[1].custom not stripped")
	}

	// Core fields preserved
	if gjson.GetBytes(result, "model").String() != "claude-haiku-4-5-20251001" {
		t.Error("model corrupted")
	}
	if gjson.GetBytes(result, "max_tokens").Int() != 1024 {
		t.Error("max_tokens corrupted")
	}
	if gjson.GetBytes(result, "system.0.text").String() != "hello" {
		t.Error("system text corrupted")
	}
	if gjson.GetBytes(result, "tools.0.name").String() != "bash" {
		t.Error("tools[0].name corrupted")
	}
	if gjson.GetBytes(result, "tools.1.name").String() != "read" {
		t.Error("tools[1].name corrupted")
	}
	if gjson.GetBytes(result, "messages.0.content.0.text").String() != "hi" {
		t.Error("messages corrupted")
	}

	// Beta header filtered
	beta := retReq.Header.Get("anthropic-beta")
	if beta != "output-128k-2025-02-19" {
		t.Errorf("expected only output-128k kept, got %q", beta)
	}
}
