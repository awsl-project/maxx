package codex

import "github.com/awsl-project/maxx/internal/domain"

// CloneForTokenPersist returns a copy of p suitable for mutating Codex token
// fields without racing concurrent lock-free readers of the shared provider.
//
// The cached ProviderRepository hands the same *domain.Provider pointer to every
// caller (GetAll/GetByID/List do not copy), and the request hot path reads token
// fields off provider.Config.Codex without a lock (e.g. Execute reads AccountID,
// getAccessToken's fast path reads AccessToken/ExpiresAt/RefreshToken). Mutating
// the shared struct in place therefore races those readers. Instead we copy the
// Provider, its Config, and its Codex config; the caller mutates the copy and
// persists it via repo.Update, which atomically swaps the cache pointer. Readers
// holding the old pointer see a consistent (if briefly stale) immutable struct.
//
// Only the Codex config is deep-copied: other Config sub-structs are shared by
// pointer (never mutated here) and the Codex ModelMapping map is shared
// (read-only here). The returned *ProviderConfigCodex is cp.Config.Codex, ready
// to mutate.
func CloneForTokenPersist(p *domain.Provider) (*domain.Provider, *domain.ProviderConfigCodex) {
	if p == nil {
		cfg := &domain.ProviderConfigCodex{}
		return &domain.Provider{Config: &domain.ProviderConfig{Codex: cfg}}, cfg
	}

	cp := *p // shallow copy of Provider

	cfgCopy := domain.ProviderConfig{}
	if p.Config != nil {
		cfgCopy = *p.Config // shallow copy of ProviderConfig
	}
	cp.Config = &cfgCopy

	codexCopy := domain.ProviderConfigCodex{}
	if p.Config != nil && p.Config.Codex != nil {
		codexCopy = *p.Config.Codex // shallow copy of Codex config
	}
	cp.Config.Codex = &codexCopy

	return &cp, &codexCopy
}
