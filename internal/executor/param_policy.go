package executor

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/reqpolicy"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// applyOutboundParamPolicy is the single authoritative stage for semantic
// outbound request-parameter policy. It runs once per attempt in dispatch, on
// the already-converted body, immediately before the body is published to the
// provider adapter — so it covers every provider type and both client-supplied
// and maxx-synthesized values. Provider adapters therefore do transport only and
// no longer mutate these parameters themselves.
//
// Order is fixed and deliberate: provider-level service_tier first, then
// provider-scoped OpenAI system prompt injection, then the reasoning clamp so the
// ceiling is always authoritative over anything a prior stage set.
//
//  1. service_tier — provider-level override (was Codex-adapter local).
//  2. OpenAI system prompt — provider-level Chat Completions message injection.
//  3. reasoning effort — DefaultEffort fill + MaxEffort clamp (reqpolicy).
func (e *Executor) applyOutboundParamPolicy(body []byte, protocol domain.ClientType, requestURI string, mappedModel string, provider *domain.Provider) []byte {
	body = applyServiceTierOverride(body, protocol, provider)
	body = applyOpenAISystemPrompt(body, protocol, requestURI, provider)
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

func applyOpenAISystemPrompt(body []byte, protocol domain.ClientType, requestURI string, provider *domain.Provider) []byte {
	if provider == nil || provider.Config == nil || protocol != domain.ClientTypeOpenAI || !isOpenAIChatCompletionsURI(requestURI) {
		return body
	}
	prompt := strings.TrimSpace(provider.Config.OpenAISystemPrompt)
	if prompt == "" {
		return body
	}

	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}

	systemMessage, err := json.Marshal(map[string]string{"role": "system", "content": prompt})
	if err != nil {
		return body
	}
	messagesRaw := strings.TrimSpace(messages.Raw)
	if !strings.HasPrefix(messagesRaw, "[") || !strings.HasSuffix(messagesRaw, "]") {
		return body
	}
	innerMessages := strings.TrimSpace(messagesRaw[1 : len(messagesRaw)-1])
	injectedMessages := "[" + string(systemMessage)
	if innerMessages != "" {
		injectedMessages += "," + innerMessages
	}
	injectedMessages += "]"

	out, err := sjson.SetRawBytes(body, "messages", []byte(injectedMessages))
	if err != nil {
		return body
	}
	return out
}

func isOpenAIChatCompletionsURI(requestURI string) bool {
	path := strings.TrimSpace(requestURI)
	if path == "" {
		return false
	}
	if u, err := url.Parse(path); err == nil && u.Path != "" {
		path = u.Path
	} else if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimRight(path, "/")
	return path == "/v1/chat/completions" || strings.HasSuffix(path, "/v1/chat/completions")
}
