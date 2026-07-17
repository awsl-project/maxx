package executor

import (
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/reqpolicy"
	"github.com/tidwall/sjson"
)

// applyOutboundParamPolicy is the single authoritative stage for semantic
// outbound request-parameter policy. It runs once per attempt in dispatch, on
// the already-converted body, immediately before the body is published to the
// provider adapter — so it covers every provider type and both client-supplied
// and maxx-synthesized values. Provider adapters therefore do transport only and
// no longer mutate these parameters themselves.
//
// Order is fixed and deliberate: provider-level service_tier first, then the
// reasoning clamp so the ceiling is always authoritative over anything a prior
// stage set.
//
//  1. service_tier — provider-level override (was Codex-adapter local).
//  2. reasoning effort — DefaultEffort fill + MaxEffort clamp (reqpolicy).
func (e *Executor) applyOutboundParamPolicy(body []byte, protocol domain.ClientType, mappedModel string, provider *domain.Provider) []byte {
	body = applyServiceTierOverride(body, protocol, provider)
	body = reqpolicy.ApplyForProvider(body, protocol, provider)
	return body
}

// applyServiceTierOverride forces service_tier from provider config. service_tier
// is an OpenAI/Codex concept, so it is only written on those outbound protocols.
func applyServiceTierOverride(body []byte, protocol domain.ClientType, provider *domain.Provider) []byte {
	if provider == nil || provider.Config == nil || provider.Config.Codex == nil {
		return body
	}
	tier := provider.Config.Codex.ServiceTier
	if tier == "" {
		return body
	}
	if protocol != domain.ClientTypeCodex && protocol != domain.ClientTypeOpenAI {
		return body
	}
	if out, err := sjson.SetBytes(body, "service_tier", tier); err == nil {
		return out
	}
	return body
}
