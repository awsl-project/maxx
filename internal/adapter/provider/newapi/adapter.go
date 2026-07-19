// Package newapi implements a first-class provider adapter for new-api / one-api
// style relay gateways (e.g. code0), a widely self-hosted aggregation gateway that
// speaks the OpenAI Chat Completions API and proxies Gemini through Google's
// OpenAI-compatibility layer. Like the openrouter adapter it is a thin wrapper over
// the proven `custom` proxy core: everything about forwarding, streaming, auth and
// error handling is identical to a custom OpenAI-compatible upstream. The only
// new-api-specific behavior is translating a client's image sizing into new-api's
// dialect (extra_body.google.image_config) before delegating — the exact mirror of
// what the openrouter adapter does for OpenRouter's own dialect.
//
// Making new-api a dedicated type (rather than a `custom` provider with a discriminator
// flag) keeps each relay product's quirks in its own adapter and lets `custom` stay a
// truly generic passthrough. See the deprecated ProviderConfigCustom.Backend field.
package newapi

import (
	"fmt"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/adapter/provider/custom"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

func init() {
	provider.RegisterAdapterFactory("newapi", NewAdapter)
}

// Adapter is a first-class new-api provider that delegates to the custom adapter,
// translating image sizing into new-api's dialect on the way out.
type Adapter struct {
	inner    provider.ProviderAdapter
	provider *domain.Provider
}

// NewAdapter builds a new-api adapter around the custom proxy core. new-api providers
// reuse the custom config (BaseURL, APIKey, model mapping, etc.) — no separate config
// shape is needed, since a new-api upstream is configured exactly like any OpenAI-
// compatible relay.
func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Custom == nil {
		return nil, fmt.Errorf("provider %s missing custom config", p.Name)
	}
	inner, err := custom.NewAdapter(p)
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner, provider: p}, nil
}

// SupportedClientTypes reports the client types this new-api provider serves.
func (a *Adapter) SupportedClientTypes() []domain.ClientType {
	return a.provider.SupportedClientTypes
}

// Execute normalizes any image sizing into new-api's extra_body.google.image_config
// form (so an OpenAI- or Gemini-origin client can express sizing its own standard
// way and still reach the upstream image model correctly), then delegates to the
// custom core.
func (a *Adapter) Execute(c *flow.Ctx, prov *domain.Provider) error {
	if body := flow.GetRequestBody(c); len(body) > 0 {
		if nb := normalizeImageConfigBody(body, flow.GetRequestURI(c)); len(nb) > 0 {
			c.Set(flow.KeyRequestBody, nb)
		}
	}
	return a.inner.Execute(c, prov)
}
