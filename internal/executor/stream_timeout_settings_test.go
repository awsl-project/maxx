package executor

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestShouldApplyOpenAIChatStreamTimeoutsOnlyOpenAIChatRoute(t *testing.T) {
	exec := &Executor{settingsRepo: &stubExecutorSettingsRepo{values: map[string]string{
		domain.SettingKeyOpenAIChatStreamTimeoutsEnabled: "true",
	}}}

	tests := []struct {
		name       string
		clientType domain.ClientType
		uri        string
		want       bool
	}{
		{name: "openai chat", clientType: domain.ClientTypeOpenAI, uri: "/v1/chat/completions", want: true},
		{name: "provider proxy openai chat", clientType: domain.ClientTypeOpenAI, uri: "/provider/7/v1/chat/completions", want: true},
		{name: "openai images", clientType: domain.ClientTypeOpenAI, uri: "/v1/images/generations", want: false},
		{name: "codex converted to openai path remains off", clientType: domain.ClientTypeCodex, uri: "/v1/chat/completions", want: false},
		{name: "claude off", clientType: domain.ClientTypeClaude, uri: "/v1/chat/completions", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exec.shouldApplyOpenAIChatStreamTimeouts(tt.clientType, tt.uri); got != tt.want {
				t.Fatalf("shouldApplyOpenAIChatStreamTimeouts(%s, %q) = %v, want %v", tt.clientType, tt.uri, got, tt.want)
			}
		})
	}
}

func TestOpenAIChatStreamTimeoutSettingsUseOpenAIChatKeys(t *testing.T) {
	exec := &Executor{settingsRepo: &stubExecutorSettingsRepo{values: map[string]string{
		domain.SettingKeyOpenAIChatStreamTimeoutsEnabled:     "true",
		domain.SettingKeyOpenAIChatStreamFirstEventTimeoutMS: "1500",
		domain.SettingKeyOpenAIChatStreamIdleTimeoutMS:       "2500",
	}}}

	if !exec.openAIChatStreamTimeoutsEnabled() {
		t.Fatal("openAIChatStreamTimeoutsEnabled() = false, want true")
	}
	if got := exec.streamFirstEventTimeout(); got != 1500*time.Millisecond {
		t.Fatalf("streamFirstEventTimeout() = %s, want 1500ms", got)
	}
	if got := exec.streamIdleTimeout(); got != 2500*time.Millisecond {
		t.Fatalf("streamIdleTimeout() = %s, want 2500ms", got)
	}
}
