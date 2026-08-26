package domain

import "testing"

func TestCanonicalSupportedClientTypes(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		configured   []ClientType
		want         []ClientType
	}{
		{
			name:         "codex ignores configured",
			providerType: "codex",
			configured:   []ClientType{ClientTypeOpenAI},
			want:         []ClientType{ClientTypeCodex},
		},
		{
			name:         "claude fixed",
			providerType: "claude",
			configured:   nil,
			want:         []ClientType{ClientTypeClaude},
		},
		{
			name:         "custom empty defaults openai",
			providerType: "custom",
			configured:   nil,
			want:         []ClientType{ClientTypeOpenAI},
		},
		{
			name:         "custom keeps configured codex",
			providerType: "custom",
			configured:   []ClientType{ClientTypeCodex},
			want:         []ClientType{ClientTypeCodex},
		},
		{
			name:         "openrouter empty defaults all three incl codex",
			providerType: "openrouter",
			configured:   nil,
			want:         []ClientType{ClientTypeClaude, ClientTypeOpenAI, ClientTypeCodex},
		},
		{
			name:         "openrouter forces codex onto pre-codex config",
			providerType: "openrouter",
			configured:   []ClientType{ClientTypeClaude, ClientTypeOpenAI},
			want:         []ClientType{ClientTypeClaude, ClientTypeOpenAI, ClientTypeCodex},
		},
		{
			name:         "openrouter keeps other configured protocols and adds codex",
			providerType: "openrouter",
			configured:   []ClientType{ClientTypeOpenAI},
			want:         []ClientType{ClientTypeOpenAI, ClientTypeCodex},
		},
		{
			name:         "openrouter does not duplicate codex",
			providerType: "openrouter",
			configured:   []ClientType{ClientTypeOpenAI, ClientTypeCodex},
			want:         []ClientType{ClientTypeOpenAI, ClientTypeCodex},
		},
		{
			name:         "zai defaults to claude, openai and codex when unset",
			providerType: "zai",
			configured:   nil,
			want:         []ClientType{ClientTypeClaude, ClientTypeOpenAI, ClientTypeCodex},
		},
		{
			name:         "zai honors configured openai-only",
			providerType: "zai",
			configured:   []ClientType{ClientTypeOpenAI},
			want:         []ClientType{ClientTypeOpenAI},
		},
		{
			name:         "zai honors configured claude-only",
			providerType: "zai",
			configured:   []ClientType{ClientTypeClaude},
			want:         []ClientType{ClientTypeClaude},
		},
		{
			name:         "antigravity fixed",
			providerType: "antigravity",
			configured:   []ClientType{ClientTypeOpenAI},
			want:         []ClientType{ClientTypeGemini, ClientTypeClaude},
		},
		{
			name:         "unknown type keeps configured",
			providerType: "responses-ws-router-native-test",
			configured:   []ClientType{ClientTypeCodex},
			want:         []ClientType{ClientTypeCodex},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanonicalSupportedClientTypes(tt.providerType, tt.configured)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestProviderNativelySupports(t *testing.T) {
	tests := []struct {
		name       string
		provider   *Provider
		clientType ClientType
		want       bool
	}{
		{
			name:       "codex provider + codex route",
			provider:   &Provider{Type: "codex"},
			clientType: ClientTypeCodex,
			want:       true,
		},
		{
			name:       "codex provider + openai route",
			provider:   &Provider{Type: "codex"},
			clientType: ClientTypeOpenAI,
			want:       false,
		},
		{
			name:       "openai provider + codex route",
			provider:   &Provider{Type: "custom", SupportedClientTypes: []ClientType{ClientTypeOpenAI}},
			clientType: ClientTypeCodex,
			want:       false,
		},
		{
			name: "custom configured codex + codex route",
			provider: &Provider{
				Type:                 "custom",
				SupportedClientTypes: []ClientType{ClientTypeCodex},
			},
			clientType: ClientTypeCodex,
			want:       true,
		},
		{
			name:       "nil provider",
			provider:   nil,
			clientType: ClientTypeCodex,
			want:       false,
		},
		{
			name: "historical codex empty supported types still native",
			provider: &Provider{
				Type:                 "codex",
				SupportedClientTypes: nil,
			},
			clientType: ClientTypeCodex,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProviderNativelySupports(tt.provider, tt.clientType); got != tt.want {
				t.Fatalf("ProviderNativelySupports = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRouteIsNative(t *testing.T) {
	codexProvider := &Provider{Type: "codex"}
	openaiProvider := &Provider{
		Type:                 "custom",
		SupportedClientTypes: []ClientType{ClientTypeOpenAI},
	}

	if !RouteIsNative(codexProvider, &Route{ClientType: ClientTypeCodex}) {
		t.Fatal("codex→codex should be native")
	}
	if RouteIsNative(codexProvider, &Route{ClientType: ClientTypeOpenAI}) {
		t.Fatal("codex→openai should not be native")
	}
	if RouteIsNative(openaiProvider, &Route{ClientType: ClientTypeCodex}) {
		t.Fatal("openai→codex should not be native")
	}
	if RouteIsNative(codexProvider, nil) {
		t.Fatal("nil route should not be native")
	}
}
