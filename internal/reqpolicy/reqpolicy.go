// Package reqpolicy is the authoritative outbound request-parameter policy layer.
//
// It is a pure function on the already-converted outbound request body, invoked
// once by the executor's dispatch stage (after cross-protocol conversion, before
// the body is handed to the provider adapter). Adapters therefore do transport
// only and never mutate semantic request parameters — there is a single place
// where reasoning-effort policy is applied, for every provider type and covering
// both client-supplied and maxx-synthesized values.
//
// Phase 1 handles reasoning effort. Effort is ordered:
//
//	none < minimal < low < medium < high
//
// plus the special "auto" (defer to provider). Two composable primitives:
//
//   - MaxEffort — a ceiling. Anything above it, "auto", or an unrecognized value
//     is clamped down to the ceiling. Clamping is idempotent (safe on retries).
//   - DefaultEffort — fills the effort only when the request carries none; it
//     never overrides a value already present.
package reqpolicy

import (
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Ordered effort ranks. "auto" is intentionally not a rank — it means "defer to
// provider" and is only meaningful relative to a ceiling (which clamps it down).
const (
	rankNone = iota
	rankMinimal
	rankLow
	rankMedium
	rankHigh
)

var effortToRank = map[string]int{
	"none":    rankNone,
	"minimal": rankMinimal,
	"low":     rankLow,
	"medium":  rankMedium,
	"high":    rankHigh,
}

var rankToEffort = map[int]string{
	rankNone:    "none",
	rankMinimal: "minimal",
	rankLow:     "low",
	rankMedium:  "medium",
	rankHigh:    "high",
}

// parseRank normalizes an effort string to its rank. ok is false for empty or
// unrecognized vocabulary (including "auto", which callers handle separately).
func parseRank(s string) (rank int, ok bool) {
	r, ok := effortToRank[strings.ToLower(strings.TrimSpace(s))]
	return r, ok
}

func isAuto(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "auto")
}

// effortValue is a decoded outbound effort field.
type effortValue struct {
	present bool // the field exists as a string
	auto    bool // value is "auto"
	hasRank bool // value is a known ordered effort
	rank    int
}

// codec hides each protocol's JSON path and value vocabulary for effort.
type codec struct {
	path   string              // gjson/sjson dotted path in the outbound body
	toWire func(string) string // rank name -> protocol wire form
}

func identity(s string) string { return s }

// codecFor returns the effort codec for an outbound protocol, or nil when the
// protocol has no simple string effort field this phase handles (e.g. Claude,
// whose depth is a thinking-token budget).
func codecFor(protocol domain.ClientType) *codec {
	switch protocol {
	case domain.ClientTypeOpenAI:
		return &codec{path: "reasoning_effort", toWire: identity}
	case domain.ClientTypeCodex:
		return &codec{path: "reasoning.effort", toWire: identity}
	case domain.ClientTypeGemini:
		// Gemini's thinkingLevel vocabulary is upper-case (LOW/MEDIUM/HIGH).
		return &codec{path: "generationConfig.thinkingConfig.thinkingLevel", toWire: strings.ToUpper}
	default:
		return nil
	}
}

func (c *codec) read(body []byte) effortValue {
	v := gjson.GetBytes(body, c.path)
	if !v.Exists() || v.Type != gjson.String {
		return effortValue{}
	}
	s := v.String()
	if strings.TrimSpace(s) == "" {
		return effortValue{}
	}
	if isAuto(s) {
		return effortValue{present: true, auto: true}
	}
	if rank, ok := parseRank(s); ok {
		return effortValue{present: true, hasRank: true, rank: rank}
	}
	return effortValue{present: true} // present but unrecognized vocabulary
}

func (c *codec) write(body []byte, rank int) []byte {
	name, ok := rankToEffort[rank]
	if !ok {
		return body
	}
	out, err := sjson.SetBytes(body, c.path, c.toWire(name))
	if err != nil {
		return body
	}
	return out
}

// Effective is a resolved policy: the composed ceiling and default a single
// Apply call enforces.
type Effective struct {
	hasMax      bool
	maxRank     int
	hasDefault  bool
	defaultRank int
}

// IsZero reports that the policy does nothing.
func (e Effective) IsZero() bool { return !e.hasMax && !e.hasDefault }

// Resolve composes scopes into an effective policy. MaxEffort composes by the
// lower ceiling across every scope that sets one. DefaultEffort is taken from
// the most specific scope that sets it: provider first, then global, then the
// legacy per-provider Codex effort (which was historically a force-override and
// is intentionally demoted to a default — it no longer overrides an explicit
// client value; operators wanting a hard bound use MaxEffort).
func Resolve(global, provider *domain.ReasoningPolicy, legacyDefault string) Effective {
	var eff Effective

	for _, p := range []*domain.ReasoningPolicy{global, provider} {
		if p == nil {
			continue
		}
		if r, ok := parseRank(p.MaxEffort); ok {
			if !eff.hasMax || r < eff.maxRank {
				eff.hasMax = true
				eff.maxRank = r
			}
		}
	}

	for _, s := range []string{provDefault(provider), globDefault(global), legacyDefault} {
		if r, ok := parseRank(s); ok {
			eff.hasDefault = true
			eff.defaultRank = r
			break
		}
	}
	return eff
}

func provDefault(p *domain.ReasoningPolicy) string {
	if p == nil {
		return ""
	}
	return p.DefaultEffort
}

func globDefault(p *domain.ReasoningPolicy) string {
	if p == nil {
		return ""
	}
	return p.DefaultEffort
}

// Apply enforces an effective effort policy on an outbound body for a protocol.
// It is a no-op when the protocol has no effort codec or the policy is zero, and
// is idempotent: re-applying never changes an already-conformant body.
func Apply(body []byte, protocol domain.ClientType, eff Effective) []byte {
	if eff.IsZero() {
		return body
	}
	c := codecFor(protocol)
	if c == nil {
		return body
	}

	v := c.read(body)

	// DefaultEffort: fill only when absent; never override an existing value.
	if !v.present && eff.hasDefault {
		body = c.write(body, eff.defaultRank)
		v = effortValue{present: true, hasRank: true, rank: eff.defaultRank}
	}

	// MaxEffort: clamp anything above the ceiling down. "auto" and unrecognized
	// vocabularies are treated as exceeding the ceiling so the bound always holds.
	if eff.hasMax && v.present {
		exceeds := v.auto || !v.hasRank || v.rank > eff.maxRank
		if exceeds {
			body = c.write(body, eff.maxRank)
		}
	}
	return body
}
