// Package ollama implements a first-class provider adapter for Ollama
// (https://ollama.com), a local/self-hosted model server that speaks its own
// /api/chat protocol. Like the openrouter and newapi adapters it is a thin wrapper
// over the `custom` proxy core, which already contains the full Claude<->Ollama
// translation (see custom/ollama.go). This adapter simply guarantees the custom
// core takes its Ollama code path.
//
// Historically Ollama was reached via a `custom` provider with Backend:"ollama".
// That still works (the field is retained for backward compatibility) but is
// deprecated in favor of this dedicated type, so `custom` can stay a generic
// passthrough and each upstream product owns its own adapter.
package ollama

import (
	"fmt"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/adapter/provider/custom"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// ollamaBackend is the custom-core backend selector that routes to the
// Claude<->Ollama translation. Kept in sync with custom.customBackendOllama.
const ollamaBackend = "ollama"

func init() {
	provider.RegisterAdapterFactory("ollama", NewAdapter)
}

// Adapter is a first-class Ollama provider that delegates to the custom core with
// the Ollama backend forced on.
type Adapter struct {
	inner provider.ProviderAdapter
	synth *domain.Provider
}

// NewAdapter builds an Ollama adapter by synthesizing a custom config with the
// Ollama backend selected, then wrapping a custom adapter around it. Ollama
// providers reuse the custom config (BaseURL, APIKey, model mapping); the caller
// need not set the deprecated Backend field — this type implies it.
func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Custom == nil {
		return nil, fmt.Errorf("provider %s missing custom config", p.Name)
	}

	// Shallow-copy the provider and its custom config so forcing the backend never
	// mutates the shared, cached provider entity.
	synth := *p
	customCfg := *p.Config.Custom
	customCfg.Backend = ollamaBackend
	cfg := *p.Config
	cfg.Custom = &customCfg
	synth.Config = &cfg

	inner, err := custom.NewAdapter(&synth)
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner, synth: &synth}, nil
}

// SupportedClientTypes reports the client types this Ollama provider serves. The
// custom Ollama path only supports Claude-compatible requests.
func (a *Adapter) SupportedClientTypes() []domain.ClientType {
	return a.synth.SupportedClientTypes
}

// Execute delegates to the custom core, passing the synthesized provider so the
// Ollama backend and its config drive the request.
func (a *Adapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	return a.inner.Execute(c, a.synth)
}
