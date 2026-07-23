package domain

import "testing"

func TestProviderResponsesWebSocketEnabled(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		provider *Provider
		want     bool
	}{
		{name: "nil", provider: nil, want: false},
		{name: "custom default off", provider: &Provider{Type: "custom"}, want: false},
		{
			name: "custom explicit on",
			provider: &Provider{Type: "custom", Config: &ProviderConfig{Custom: &ProviderConfigCustom{
				ResponsesWebSocket: &trueVal,
			}}},
			want: true,
		},
		{
			name: "custom explicit off",
			provider: &Provider{Type: "custom", Config: &ProviderConfig{Custom: &ProviderConfigCustom{
				ResponsesWebSocket: &falseVal,
			}}},
			want: false,
		},
		{name: "codex default on", provider: &Provider{Type: "codex"}, want: true},
		{
			name: "codex cliproxy default off",
			provider: &Provider{Type: "codex", Config: &ProviderConfig{Codex: &ProviderConfigCodex{
				UseCLIProxyAPI: true,
			}}},
			want: false,
		},
		{
			name: "codex cliproxy explicit on",
			provider: &Provider{Type: "codex", Config: &ProviderConfig{Codex: &ProviderConfigCodex{
				UseCLIProxyAPI:     true,
				ResponsesWebSocket: &trueVal,
			}}},
			want: true,
		},
		// Non-custom types default on; adapter interface still gates real eligibility.
		{name: "unknown type default on", provider: &Provider{Type: "responses-ws-router-native-test"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProviderResponsesWebSocketEnabled(tt.provider); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
