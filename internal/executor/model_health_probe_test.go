package executor

import (
	"encoding/json"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestBuildModelHealthProbeRequestUsesSourceClientType(t *testing.T) {
	tests := []struct {
		name       string
		clientType domain.ClientType
		wantURI    string
		wantField  string
	}{
		{name: "openai", clientType: domain.ClientTypeOpenAI, wantURI: "/v1/chat/completions", wantField: "messages"},
		{name: "claude", clientType: domain.ClientTypeClaude, wantURI: "/v1/messages", wantField: "messages"},
		{name: "codex", clientType: domain.ClientTypeCodex, wantURI: "/responses", wantField: "input"},
		{name: "gemini", clientType: domain.ClientTypeGemini, wantURI: "/v1beta/models/probe-model:generateContent", wantField: "contents"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, body, err := buildModelHealthProbeRequest(tt.clientType, "probe-model")
			if err != nil {
				t.Fatalf("buildModelHealthProbeRequest: %v", err)
			}
			if uri != tt.wantURI {
				t.Fatalf("uri = %q, want %q", uri, tt.wantURI)
			}
			var decoded map[string]any
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if _, ok := decoded[tt.wantField]; !ok {
				t.Fatalf("body = %s, missing %q", body, tt.wantField)
			}
		})
	}
}
