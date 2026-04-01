package codex

import (
	"fmt"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

func ensureCodexConfig(p *domain.Provider) *domain.ProviderConfigCodex {
	if p.Config == nil {
		p.Config = &domain.ProviderConfig{}
	}
	if p.Config.Codex == nil {
		p.Config.Codex = &domain.ProviderConfigCodex{}
	}
	return p.Config.Codex
}

func buildFallbackCodexAccessToken(p *domain.Provider) string {
	name := "local"
	if p != nil && strings.TrimSpace(p.Name) != "" {
		name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(p.Name), " ", "-"))
	}
	return fmt.Sprintf("maxx-local-%s", name)
}
