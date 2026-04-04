package error_fixer

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func TestExtraBodyFieldsFixer_MatchResponse(t *testing.T) {
	f := &extraBodyFieldsFixer{}

	tests := []struct {
		name       string
		resp       *http.Response
		body       string
		clientType domain.ClientType
		want       bool
	}{
		{
			name:       "output_config rejected",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"output_config.effort: Extra inputs are not permitted"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "context_management rejected",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"context_management: Extra inputs are not permitted"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "reasoning rejected",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"reasoning: Extra inputs are not permitted"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "unrelated 400 error",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"invalid model"}}`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "field name without Extra inputs pattern — no match",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"invalid reasoning format"}}`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "nil response with Extra inputs — SSE path",
			resp:       nil,
			body:       `output_config: Extra inputs are not permitted`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "wrong client type",
			resp:       &http.Response{StatusCode: 400},
			body:       `output_config: Extra inputs are not permitted`,
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

func TestExtraBodyFieldsFixer_FixRequest(t *testing.T) {
	f := &extraBodyFieldsFixer{}

	input := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"output_config": {"effort": "high"},
		"context_management": {"truncation": "auto"},
		"reasoning": {"budget_tokens": 5000},
		"messages": [{"role": "user", "content": "hello"}]
	}`)

	req, _ := http.NewRequest("POST", "https://example.com", nil)
	_, result := f.FixRequest(req, input)

	// Stripped fields
	if gjson.GetBytes(result, "output_config").Exists() {
		t.Error("output_config not stripped")
	}
	if gjson.GetBytes(result, "context_management").Exists() {
		t.Error("context_management not stripped")
	}
	if gjson.GetBytes(result, "reasoning").Exists() {
		t.Error("reasoning not stripped")
	}

	// Preserved fields
	if gjson.GetBytes(result, "model").String() != "claude-sonnet-4-20250514" {
		t.Error("model was corrupted")
	}
	if gjson.GetBytes(result, "max_tokens").Int() != 1024 {
		t.Error("max_tokens was corrupted")
	}
	if gjson.GetBytes(result, "messages.0.content").String() != "hello" {
		t.Error("messages was corrupted")
	}
}

func TestExtraBodyFieldsFixer_FixRequest_Partial(t *testing.T) {
	f := &extraBodyFieldsFixer{}

	// Only output_config present, others absent
	input := []byte(`{"model":"claude-sonnet-4-20250514","output_config":{"effort":"low"}}`)

	req, _ := http.NewRequest("POST", "https://example.com", nil)
	_, result := f.FixRequest(req, input)

	if gjson.GetBytes(result, "output_config").Exists() {
		t.Error("output_config not stripped")
	}
	if gjson.GetBytes(result, "model").String() != "claude-sonnet-4-20250514" {
		t.Error("model was corrupted")
	}
}
