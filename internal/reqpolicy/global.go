package reqpolicy

import (
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
)

var (
	globalGetterMu sync.RWMutex
	globalGetter   func() (*domain.ReasoningPolicy, error)
)

// SetGlobalPolicyGetter wires the source of the global (system-wide) reasoning
// policy. Mirrors the payloadoverride/converter settings-getter pattern so the
// core layer owns persistence and this package stays dependency-light.
func SetGlobalPolicyGetter(getter func() (*domain.ReasoningPolicy, error)) {
	globalGetterMu.Lock()
	defer globalGetterMu.Unlock()
	globalGetter = getter
}

func globalPolicy() *domain.ReasoningPolicy {
	globalGetterMu.RLock()
	getter := globalGetter
	globalGetterMu.RUnlock()
	if getter == nil {
		return nil
	}
	p, err := getter()
	if err != nil {
		return nil
	}
	return p
}

// ApplyForProvider resolves the effective reasoning policy for a provider
// (global ⊓ provider ceiling, most-specific default, legacy Codex effort demoted
// to a default) and enforces it on the outbound body. No-op when nothing applies.
func ApplyForProvider(body []byte, protocol domain.ClientType, provider *domain.Provider) []byte {
	if provider == nil {
		return body
	}

	var provPolicy *domain.ReasoningPolicy
	legacy := ""
	if provider.Config != nil {
		provPolicy = provider.Config.Reasoning
		if provider.Config.Codex != nil {
			legacy = provider.Config.Codex.Reasoning
		}
	}

	eff := Resolve(globalPolicy(), provPolicy, legacy)
	return Apply(body, protocol, eff)
}
