// Package zai implements a first-class provider adapter for z.ai (智谱 / Zhipu
// GLM, https://z.ai) — the vendor of the GLM model family (glm-4.6, glm-4.5,
// glm-4.5-air, ...). z.ai exposes THREE compatible protocols over the same key:
//
//   - Anthropic Messages    https://api.z.ai/api/anthropic          (Claude clients,
//     /v1/messages) — exactly the endpoint the z.ai "GLM coding plan" points Claude
//     Code at (ANTHROPIC_BASE_URL=.../api/anthropic, ANTHROPIC_AUTH_TOKEN=<key>).
//   - OpenAI Chat Completions .../paas/v4                            (OpenAI clients)
//   - OpenAI Responses      https://api.z.ai/api/v1/responses        (Codex clients) —
//     plan-independent; the endpoint Codex CLI drives.
//
// z.ai further splits into two "ports"/plans that differ ONLY in the OpenAI root:
//
//   - "coding" (编程套餐, default): OpenAI → https://api.z.ai/api/coding/paas/v4
//   - "api"    (标准 API/资源包):   OpenAI → https://api.z.ai/api/paas/v4
//
// The Anthropic endpoint is identical for both plans. Because both protocols are
// what the generic `custom` adapter already proxies, this adapter is a thin
// wrapper — mirroring the openrouter/newapi adapters: it synthesizes a
// ProviderConfigCustom that maps each client type to the right z.ai base URL
// (via ClientBaseURL) plus the user's API key, then delegates every request to
// the proven custom proxy core. This keeps z.ai a real, dedicated provider type
// (own config, own UI, own icon) without duplicating the proxy/streaming logic.
package zai

import (
	"fmt"
	"os"
	"strings"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/adapter/provider/custom"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// z.ai's fixed API roots. The Anthropic root is shared by both plans; the OpenAI
// root depends on the plan (coding vs standard API).
const (
	// defaultBaseURL is z.ai's Anthropic-compatible root (Claude clients append
	// /v1/messages). Kept as the exported default and the synthesized fallback
	// BaseURL.
	defaultBaseURL = "https://api.z.ai/api/anthropic"

	// openAICodingBaseURL is the OpenAI Chat Completions root for the GLM Coding
	// Plan (编程套餐). NOTE the mandatory /coding/ segment — z.ai rejects the plain
	// /paas/v4 root for coding-plan keys.
	openAICodingBaseURL = "https://api.z.ai/api/coding/paas/v4"

	// openAIAPIBaseURL is the OpenAI Chat Completions root for the standard API
	// (标准 API / 资源包).
	openAIAPIBaseURL = "https://api.z.ai/api/paas/v4"

	// openAIResponsesBaseURL is z.ai's OpenAI Responses API root (Codex clients).
	// It is plan-independent: z.ai serves Responses at /api/v1/responses for both
	// the coding plan and the standard API (the /coding/paas/v4 and /paas/v4 roots
	// 404 on /responses). Codex clients append /responses (the custom core echoes
	// the client's original Responses path; buildUpstreamURL collapses the extra
	// /v1 against this versioned root).
	openAIResponsesBaseURL = "https://api.z.ai/api/v1"

	// planCoding is the default plan (GLM Coding Plan); planAPI is the standard API.
	planCoding = "coding"
	planAPI    = "api"
)

// envBaseURL returns a trimmed base-URL override from the named env var, or ""
// when unset. Trailing slashes are stripped so downstream path concatenation
// never doubles the separator — buildUpstreamURL trims too, but the admin
// model-list/benchmark helpers concatenate OpenAIBaseURL(plan) with "/models" /
// "/chat/completions" directly, where a stray "https://.../paas/v4/" would
// otherwise yield ".../v4//models" (some upstreams 404 on that).
func envBaseURL(key string) string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/")
}

// resolveBaseURL returns z.ai's Anthropic API root (Claude clients). Fixed in
// production but redirectable via MAXX_ZAI_BASE_URL — used by tests to point at a
// mock upstream, and usable to target the China mainland root
// (https://open.bigmodel.cn/api/anthropic) or a self-hosted GLM gateway.
func resolveBaseURL() string {
	if v := envBaseURL("MAXX_ZAI_BASE_URL"); v != "" {
		return v
	}
	return defaultBaseURL
}

// normalizePlan folds an empty/unknown plan to the default "coding". Any value
// other than "api" is treated as coding so a stray config can never silently
// point coding-plan keys at the standard-API root (or vice versa in a way that
// 404s).
func normalizePlan(plan string) string {
	if strings.EqualFold(strings.TrimSpace(plan), planAPI) {
		return planAPI
	}
	return planCoding
}

// resolveOpenAIBaseURL returns z.ai's OpenAI Chat Completions root for the given
// plan (OpenAI clients). Redirectable via MAXX_ZAI_OPENAI_BASE_URL (e.g. to the
// China mainland root https://open.bigmodel.cn/api/[coding/]paas/v4 or a mock
// upstream); the override wins for both plans.
func resolveOpenAIBaseURL(plan string) string {
	if v := envBaseURL("MAXX_ZAI_OPENAI_BASE_URL"); v != "" {
		return v
	}
	if normalizePlan(plan) == planAPI {
		return openAIAPIBaseURL
	}
	return openAICodingBaseURL
}

// OpenAIBaseURL returns z.ai's OpenAI Chat Completions root for the given plan.
// Exported so admin/test-field handlers (model discovery, benchmark) hit the
// plan-correct endpoint — a coding-plan key 404s on the standard-API root
// (/api/paas/v4) and a standard key can't use the coding root. Callers append
// /models or /chat/completions.
func OpenAIBaseURL(plan string) string {
	return resolveOpenAIBaseURL(plan)
}

// resolveResponsesBaseURL returns z.ai's OpenAI Responses root (Codex clients).
// Redirectable via MAXX_ZAI_RESPONSES_BASE_URL (e.g. the China mainland root
// https://open.bigmodel.cn/api/v1 or a mock upstream). Plan-independent.
func resolveResponsesBaseURL() string {
	if v := envBaseURL("MAXX_ZAI_RESPONSES_BASE_URL"); v != "" {
		return v
	}
	return openAIResponsesBaseURL
}

func init() {
	provider.RegisterAdapterFactory("zai", NewAdapter)
}

// Adapter is a first-class z.ai provider that delegates to the custom adapter
// using a synthesized custom config.
type Adapter struct {
	inner provider.ProviderAdapter
	// synth is the provider carrying the synthesized Custom config. It is passed
	// to the inner adapter's Execute so no code path ever dereferences the real
	// provider's (nil) Custom config.
	synth *domain.Provider
}

// NewAdapter builds a z.ai adapter by translating the z.ai config into an
// equivalent custom config and wrapping a custom adapter around it.
func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Zai == nil {
		return nil, fmt.Errorf("provider %s missing zai config", p.Name)
	}
	z := p.Config.Zai

	// Shallow-copy the provider and swap in a synthesized custom config so the
	// custom core sees a normal "custom" provider pointed at z.ai. BaseURL/APIKey
	// and the per-client ClientBaseURL map are consumed by the custom adapter at
	// request time; model mapping is handled generically via ModelMapping entities
	// keyed by provider id (executor.mapModel), so nothing model-related lives here.
	//
	// ClientBaseURL routes each protocol to its own z.ai root: Claude → the shared
	// Anthropic endpoint; OpenAI → the plan-specific Chat Completions endpoint. The
	// top-level BaseURL is the Anthropic root as a safe fallback for any client type
	// without an explicit entry (getBaseURL falls back to it). Populating both keys
	// unconditionally is fine — the custom core only reads the entry matching the
	// inbound request's client type, and routing already gates which clients reach
	// this provider (SupportedClientTypes).
	anthropicBase := resolveBaseURL()
	openAIBase := resolveOpenAIBaseURL(z.Plan)
	responsesBase := resolveResponsesBaseURL()
	synth := *p
	synth.Config = &domain.ProviderConfig{
		DisableErrorCooldown: p.Config.DisableErrorCooldown,
		Custom: &domain.ProviderConfigCustom{
			BaseURL: anthropicBase,
			APIKey:  z.APIKey,
			ClientBaseURL: map[domain.ClientType]string{
				domain.ClientTypeClaude: anthropicBase,
				domain.ClientTypeOpenAI: openAIBase,
				// Codex speaks the OpenAI Responses API; z.ai serves it at
				// /api/v1/responses regardless of plan. Responses passthrough is on by
				// default (ResponsesPassthrough nil → true), so the custom core echoes
				// the client's original path and buildUpstreamURL collapses the extra
				// /v1 against this versioned root.
				domain.ClientTypeCodex: responsesBase,
			},
			// z.ai's endpoints are first-party, natively compatible gateways — the
			// Anthropic one must NOT be disguised as the Claude Code CLI. The custom
			// adapter treats a nil Disguise as claude-code (injecting a system prompt,
			// prompt-caching cache_control, forced x-api-key, etc.); forcing "none"
			// passes the client's original request through untouched (auth aside) so
			// GLM sees exactly what the client sent, and setClaudeAuthForURL writes
			// Authorization: Bearer <key> (z.ai is a non-anthropic host), which is how
			// z.ai authenticates. Disguise only affects the Claude client path; OpenAI
			// clients are forwarded with a Bearer key regardless.
			Disguise: &domain.ProviderConfigCustomDisguise{Type: domain.DisguiseTypeNone},
		},
	}

	inner, err := custom.NewAdapter(&synth)
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner, synth: &synth}, nil
}

// SupportedClientTypes reports the client types this z.ai provider serves —
// whichever of Claude/OpenAI the provider was configured with (defaulting to
// both), reconciled against the canonical native capability set rather than the
// possibly stale stored list. See domain.CanonicalSupportedClientTypes.
func (a *Adapter) SupportedClientTypes() []domain.ClientType {
	return domain.CanonicalSupportedClientTypes("zai", a.synth.SupportedClientTypes)
}

// Execute delegates to the custom core, passing the synthesized provider so the
// custom config drives base URL and auth. GLM model ids are reached through the
// generic ModelMapping (executor.mapModel rewrites flow's mapped model before we
// run), so no z.ai-specific model or body rewriting is needed here.
func (a *Adapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	return a.inner.Execute(c, a.synth)
}
