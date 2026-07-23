package flow

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestResolveUpstreamUserAgent(t *testing.T) {
	tests := []struct {
		name         string
		originalType domain.ClientType
		targetType   domain.ClientType
		clientUA     string
		targetUA     string
		want         string
	}{
		{"same protocol preserves arbitrary UA", domain.ClientTypeCodex, domain.ClientTypeCodex, "custom-client/1.0", "codex/default", "custom-client/1.0"},
		{"same protocol preserves missing UA", domain.ClientTypeClaude, domain.ClientTypeClaude, "", "claude/default", ""},
		{"conversion uses target UA", domain.ClientTypeClaude, domain.ClientTypeGemini, "claude-cli/source", "gemini/default", "gemini/default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, "http://localhost", nil)
			if tt.clientUA != "" {
				req.Header.Set("User-Agent", tt.clientUA)
			}
			ctx := NewCtx(nil, req)
			ctx.Set(KeyOriginalClientType, tt.originalType)
			ctx.Set(KeyClientType, tt.targetType)

			if got := ResolveUpstreamUserAgent(ctx, tt.targetUA); got != tt.want {
				t.Fatalf("ResolveUpstreamUserAgent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsProtocolConversion(t *testing.T) {
	ctx := NewCtx(nil, nil)
	ctx.Set(KeyOriginalClientType, domain.ClientTypeClaude)
	ctx.Set(KeyClientType, domain.ClientTypeCodex)
	if !IsProtocolConversion(ctx) {
		t.Fatal("expected different source and target protocols to be a conversion")
	}

	ctx.Set(KeyOriginalClientType, domain.ClientTypeCodex)
	if IsProtocolConversion(ctx) {
		t.Fatal("expected matching source and target protocols not to be a conversion")
	}
}
