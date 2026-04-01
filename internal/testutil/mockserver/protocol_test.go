package mockserver

import (
	"net/http"
	"testing"
)

func TestDetectProtocol(t *testing.T) {
	tests := []struct {
		path     string
		expected Protocol
	}{
		{"/v1/messages", ProtocolClaude},
		{"/v1/messages/", ProtocolClaude},
		{"/v1/chat/completions", ProtocolOpenAI},
		{"/v1beta/models/gemini-2.5-pro:generateContent", ProtocolGemini},
		{"/v1beta/models/gemini-2.5-pro:streamGenerateContent", ProtocolGemini},
		{"/v1/responses", ProtocolCodex},
		{"/responses", ProtocolCodex},
		{"/unknown", ProtocolOpenAI}, // fallback
	}
	for _, tt := range tests {
		req, _ := http.NewRequest("POST", "http://localhost"+tt.path, nil)
		got := DetectProtocol(req)
		if got != tt.expected {
			t.Errorf("DetectProtocol(%q) = %q, want %q", tt.path, got, tt.expected)
		}
	}
}

func TestExtractModel(t *testing.T) {
	tests := []struct {
		name     string
		protocol Protocol
		body     string
		urlPath  string
		expected string
	}{
		{"openai", ProtocolOpenAI, `{"model":"gpt-4o","messages":[]}`, "", "gpt-4o"},
		{"claude", ProtocolClaude, `{"model":"claude-sonnet-4","messages":[]}`, "", "claude-sonnet-4"},
		{"gemini from url", ProtocolGemini, `{"contents":[]}`, "/v1beta/models/gemini-2.5-pro:generateContent", "gemini-2.5-pro"},
		{"gemini stream url", ProtocolGemini, `{"contents":[]}`, "/v1beta/models/gemini-2.5-flash:streamGenerateContent", "gemini-2.5-flash"},
		{"codex", ProtocolCodex, `{"model":"gpt-4o","input":"hello"}`, "", "gpt-4o"},
		{"no model field", ProtocolOpenAI, `{"messages":[]}`, "", "mock-model"},
		{"gemini no url match", ProtocolGemini, `{}`, "/other/path", "gemini-2.5-flash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractModel(tt.protocol, []byte(tt.body), tt.urlPath)
			if got != tt.expected {
				t.Errorf("ExtractModel() = %q, want %q", got, tt.expected)
			}
		})
	}
}
