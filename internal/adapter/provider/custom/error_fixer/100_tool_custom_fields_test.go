package error_fixer

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func TestToolCustomFieldsFixer_MatchResponse(t *testing.T) {
	f := &toolCustomFieldsFixer{}

	tests := []struct {
		name       string
		resp       *http.Response
		body       string
		clientType domain.ClientType
		want       bool
	}{
		{
			name:       "defer_loading rejected",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"tools.0.custom.defer_loading: Extra inputs are not permitted"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "eager_input_streaming rejected",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"tools.0.custom.eager_input_streaming: Extra inputs are not permitted"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "input_examples rejected",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"tools.3.custom.input_examples: Extra inputs are not permitted"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "unrelated tool error",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"tools.0.name: field required"}}`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "generic .custom. field error",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"tools.0.custom.new_future_field: Extra inputs are not permitted"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "nil response SSE path",
			resp:       nil,
			body:       `tools.0.custom.defer_loading: Extra inputs are not permitted`,
			clientType: domain.ClientTypeClaude,
			want:       true,
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

func TestToolCustomFieldsFixer_FixRequest(t *testing.T) {
	f := &toolCustomFieldsFixer{}

	input := []byte(`{
		"tools": [
			{
				"name": "bash",
				"description": "Run a command",
				"custom": {"defer_loading": true, "eager_input_streaming": true},
				"input_schema": {"type": "object"}
			},
			{
				"name": "read",
				"description": "Read a file",
				"custom": {"input_examples": [{"path": "/tmp"}]}
			},
			{
				"name": "clean_tool",
				"description": "No custom field"
			}
		]
	}`)

	req, _ := http.NewRequest("POST", "https://example.com", nil)
	_, result := f.FixRequest(req, input)

	// custom stripped from tools that had it
	if gjson.GetBytes(result, "tools.0.custom").Exists() {
		t.Error("tools[0].custom not stripped")
	}
	if gjson.GetBytes(result, "tools.1.custom").Exists() {
		t.Error("tools[1].custom not stripped")
	}

	// Other fields preserved
	if gjson.GetBytes(result, "tools.0.name").String() != "bash" {
		t.Error("tools[0].name was corrupted")
	}
	if gjson.GetBytes(result, "tools.0.input_schema.type").String() != "object" {
		t.Error("tools[0].input_schema was corrupted")
	}
	if gjson.GetBytes(result, "tools.1.name").String() != "read" {
		t.Error("tools[1].name was corrupted")
	}
	if gjson.GetBytes(result, "tools.2.name").String() != "clean_tool" {
		t.Error("tools[2].name was corrupted")
	}
}

func TestToolCustomFieldsFixer_FixRequest_NoTools(t *testing.T) {
	f := &toolCustomFieldsFixer{}

	input := []byte(`{"messages": [{"role": "user", "content": "hi"}]}`)

	req, _ := http.NewRequest("POST", "https://example.com", nil)
	_, result := f.FixRequest(req, input)

	if gjson.GetBytes(result, "messages.0.content").String() != "hi" {
		t.Error("body was corrupted")
	}
}
