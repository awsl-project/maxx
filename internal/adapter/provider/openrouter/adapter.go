// Package openrouter implements a first-class provider adapter for OpenRouter
// (https://openrouter.ai), an aggregation gateway that natively speaks both the
// OpenAI Chat Completions API (/api/v1/chat/completions) and the Anthropic
// Messages API ("Anthropic Skin", /api/v1/messages). Because both endpoints are
// exactly what the generic `custom` adapter already proxies, this adapter is a
// thin wrapper: it synthesizes a ProviderConfigCustom with the fixed OpenRouter
// base URL and the user's API key, then delegates every request to the proven
// custom proxy core. This keeps OpenRouter a real, dedicated provider type
// (own config, own UI, own icon) without duplicating the proxy/streaming logic.
package openrouter

import (
	"fmt"
	"os"
	"strings"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/adapter/provider/custom"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// defaultBaseURL is OpenRouter's fixed API root. Clients append their own path:
// claude clients hit /v1/messages, openai clients hit /v1/chat/completions.
const defaultBaseURL = "https://openrouter.ai/api"

// resolveBaseURL returns the OpenRouter API root. It is fixed in production but
// can be redirected via MAXX_OPENROUTER_BASE_URL — used by tests to point at a
// mock upstream, and usable to target a self-hosted OpenRouter-compatible gateway.
func resolveBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("MAXX_OPENROUTER_BASE_URL")); v != "" {
		return v
	}
	return defaultBaseURL
}

func init() {
	provider.RegisterAdapterFactory("openrouter", NewAdapter)
}

// Adapter is a first-class OpenRouter provider that delegates to the custom
// adapter using a synthesized custom config.
type Adapter struct {
	inner provider.ProviderAdapter
	// synth is the provider carrying the synthesized Custom config. It is passed
	// to the inner adapter's Execute so no code path ever dereferences the real
	// provider's (nil) Custom config.
	synth *domain.Provider
}

// NewAdapter builds an OpenRouter adapter by translating the OpenRouter config
// into an equivalent custom config and wrapping a custom adapter around it.
func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.OpenRouter == nil {
		return nil, fmt.Errorf("provider %s missing openrouter config", p.Name)
	}
	or := p.Config.OpenRouter

	// Shallow-copy the provider and swap in a synthesized custom config so the
	// custom core sees a normal "custom" provider pointed at OpenRouter. Only
	// BaseURL and APIKey are consumed by the custom adapter at request time;
	// model mapping is handled generically via ModelMapping entities keyed by
	// provider id (executor.mapModel), so nothing model-related lives here.
	synth := *p
	synth.Config = &domain.ProviderConfig{
		DisableErrorCooldown: p.Config.DisableErrorCooldown,
		Custom: &domain.ProviderConfigCustom{
			BaseURL: resolveBaseURL(),
			APIKey:  or.APIKey,
			// OpenRouter is a first-party API gateway that natively speaks the
			// OpenAI and Anthropic protocols — it must NOT be disguised as the
			// Claude Code CLI. The custom adapter treats a nil Disguise as
			// claude-code (injecting a system prompt, prompt-caching cache_control,
			// etc.), which makes OpenRouter's Bedrock-backed Anthropic models 400
			// with "request did not allow prompt caching". Force "none" so the
			// client's original request passes through untouched (auth aside).
			Disguise: &domain.ProviderConfigCustomDisguise{Type: domain.DisguiseTypeNone},
		},
	}

	inner, err := custom.NewAdapter(&synth)
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner, synth: &synth}, nil
}

// SupportedClientTypes reports the client types this OpenRouter provider serves
// (typically claude + openai, both natively supported by OpenRouter).
func (a *Adapter) SupportedClientTypes() []domain.ClientType {
	return a.synth.SupportedClientTypes
}

// Execute delegates to the custom core, passing the synthesized provider so the
// custom config drives base URL and auth.
func (a *Adapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	return a.inner.Execute(c, a.synth)
}
