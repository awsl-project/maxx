package codex

import (
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

// codexStreamError holds an SSE-emitted error signal we observed during the
// Responses API stream. Used to refine the scope of a "stream closed before
// response.completed" failure: an in-band model_not_supported event should
// freeze only that model, not the whole provider.
type codexStreamError struct {
	typeStr string
	code    string
	message string
	model   string // upstream-reported model when the event carries it
}

// parseCodexStreamErrorLine inspects a single SSE line and returns a captured
// error if the line is a structured error event. Other lines return nil.
//
// Handles three observed wire shapes:
//   - Responses API: data: {"type":"response.failed","response":{"error":{"code":...,"message":...}}}
//   - Generic SSE error: data: {"type":"error","code":...,"message":...}
//   - FastAPI-style detail: data: {"detail":"..."} (no type) — seen on
//     "model not supported when using Codex with a ChatGPT account"
func parseCodexStreamErrorLine(line string) *codexStreamError {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" || data == "[DONE]" || !gjson.Valid(data) {
		return nil
	}

	switch t := gjson.Get(data, "type").String(); t {
	case "response.failed", "response.error":
		return &codexStreamError{
			typeStr: t,
			code:    gjson.Get(data, "response.error.code").String(),
			message: gjson.Get(data, "response.error.message").String(),
			model:   gjson.Get(data, "response.model").String(),
		}
	case "error":
		return &codexStreamError{
			typeStr: t,
			code:    gjson.Get(data, "code").String(),
			message: gjson.Get(data, "message").String(),
			model:   gjson.Get(data, "model").String(),
		}
	case "":
		if detail := gjson.Get(data, "detail").String(); detail != "" {
			return &codexStreamError{message: detail}
		}
	}
	return nil
}

// classifyCodexStreamError maps a captured error event to a refined
// (scope, reason). Returns ok=false when the event does not match any known
// signal — the caller should keep the default ScopeProvider/NetworkError.
//
// Patterns are kept narrow on purpose: only classify when the upstream's
// signal is unambiguous. Anything we can't read confidently stays at
// ScopeProvider so genuine outages still trip the wider cooldown.
func classifyCodexStreamError(e *codexStreamError) (domain.ErrorScope, domain.CooldownReason, bool) {
	if e == nil {
		return "", "", false
	}
	code := strings.ToLower(e.code)
	msg := strings.ToLower(e.message)

	// Model-level: do not freeze unrelated models on the same provider.
	if code == "model_not_found" || code == "model_not_supported" {
		return domain.ScopeModel, domain.CooldownReasonModelUnavailable, true
	}
	for _, p := range []string{
		"model is not supported",
		"model not supported",
		"model is not available",
		"model not available",
		"no access to the model",
		"does not have access to model",
		"model_not_found",
		"does not exist or you do not have access",
	} {
		if strings.Contains(msg, p) {
			return domain.ScopeModel, domain.CooldownReasonModelUnavailable, true
		}
	}

	// Key-level: bad token / account scope, no point retrying on the same key.
	if code == "invalid_api_key" || code == "unauthorized" || code == "permission_denied" {
		return domain.ScopeKey, domain.CooldownReasonAuthFailure, true
	}
	for _, p := range []string{
		"invalid api key",
		"unauthorized",
		"authentication failed",
	} {
		if strings.Contains(msg, p) {
			return domain.ScopeKey, domain.CooldownReasonAuthFailure, true
		}
	}

	// Rate-limit / quota: short cooldown per key.
	if code == "rate_limit_exceeded" || strings.Contains(msg, "rate limit") {
		return domain.ScopeKey, domain.CooldownReasonRateLimitExceeded, true
	}
	if code == "insufficient_quota" || code == "billing_hard_limit_reached" {
		return domain.ScopeKey, domain.CooldownReasonQuotaExhausted, true
	}
	for _, p := range []string{
		"insufficient quota",
		"quota exceeded",
		"exceeded your current quota",
		"exceeded your quota",
		"out of quota",
		"exhausted your quota",
	} {
		if strings.Contains(msg, p) {
			return domain.ScopeKey, domain.CooldownReasonQuotaExhausted, true
		}
	}

	return "", "", false
}
