package codex

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestParseCodexStreamErrorLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantNil  bool
		wantType string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "response.failed with nested error",
			line:     `data: {"type":"response.failed","response":{"error":{"code":"model_not_supported","message":"gpt-5.5-codex is not supported"}}}`,
			wantType: "response.failed",
			wantCode: "model_not_supported",
			wantMsg:  "gpt-5.5-codex is not supported",
		},
		{
			name:     "response.error with nested error",
			line:     `data: {"type":"response.error","response":{"error":{"code":"rate_limit_exceeded","message":"slow down"}}}`,
			wantType: "response.error",
			wantCode: "rate_limit_exceeded",
			wantMsg:  "slow down",
		},
		{
			name:     "generic error event",
			line:     `data: {"type":"error","code":"insufficient_quota","message":"out of credit"}`,
			wantType: "error",
			wantCode: "insufficient_quota",
			wantMsg:  "out of credit",
		},
		{
			name:    "fastapi detail without type",
			line:    `data: {"detail":"The 'gpt-5.5-codex' model is not supported when using Codex with a ChatGPT account."}`,
			wantMsg: "The 'gpt-5.5-codex' model is not supported when using Codex with a ChatGPT account.",
		},
		{
			name:    "non-error event ignored",
			line:    `data: {"type":"response.output_text.delta","delta":"hello"}`,
			wantNil: true,
		},
		{
			name:    "completed event ignored",
			line:    `data: {"type":"response.completed","response":{}}`,
			wantNil: true,
		},
		{
			name:    "DONE marker",
			line:    `data: [DONE]`,
			wantNil: true,
		},
		{
			name:    "non-data line",
			line:    `event: response.failed`,
			wantNil: true,
		},
		{
			name:    "invalid JSON",
			line:    `data: not json`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCodexStreamErrorLine(tt.line)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil")
			}
			if got.typeStr != tt.wantType {
				t.Errorf("type = %q, want %q", got.typeStr, tt.wantType)
			}
			if got.code != tt.wantCode {
				t.Errorf("code = %q, want %q", got.code, tt.wantCode)
			}
			if got.message != tt.wantMsg {
				t.Errorf("message = %q, want %q", got.message, tt.wantMsg)
			}
		})
	}
}

func TestClassifyCodexStreamError(t *testing.T) {
	tests := []struct {
		name       string
		err        *codexStreamError
		wantScope  domain.ErrorScope
		wantReason domain.CooldownReason
		wantOK     bool
	}{
		{
			name:   "nil event",
			err:    nil,
			wantOK: false,
		},
		{
			name:       "model_not_supported by code",
			err:        &codexStreamError{code: "model_not_supported"},
			wantScope:  domain.ScopeModel,
			wantReason: domain.CooldownReasonModelUnavailable,
			wantOK:     true,
		},
		{
			name:       "ChatGPT-account model error by message",
			err:        &codexStreamError{message: "The 'gpt-5.5-codex' model is not supported when using Codex with a ChatGPT account."},
			wantScope:  domain.ScopeModel,
			wantReason: domain.CooldownReasonModelUnavailable,
			wantOK:     true,
		},
		{
			name:       "openai model-does-not-exist message",
			err:        &codexStreamError{message: "The model `foobar` does not exist or you do not have access to it"},
			wantScope:  domain.ScopeModel,
			wantReason: domain.CooldownReasonModelUnavailable,
			wantOK:     true,
		},
		{
			// Guard against the previous over-broad "unknown model" pattern:
			// a tool/function-argument validation message that happens to
			// mention "unknown model" should NOT cool a working model down.
			name:   "tool validation mentioning unknown model is not a model cooldown",
			err:    &codexStreamError{message: "function_call: unknown model parameter in tool spec"},
			wantOK: false,
		},
		{
			// Guard against the previous over-broad bare "quota" substring:
			// a per-conversation budget message should NOT cool the key down.
			name:   "conversation context quota is not a key cooldown",
			err:    &codexStreamError{message: "context quota for this conversation exceeded"},
			wantOK: false,
		},
		{
			name:       "openai quota-exceeded message",
			err:        &codexStreamError{message: "You exceeded your current quota, please check your plan"},
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonQuotaExhausted,
			wantOK:     true,
		},
		{
			name:       "rate limit",
			err:        &codexStreamError{code: "rate_limit_exceeded"},
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonRateLimitExceeded,
			wantOK:     true,
		},
		{
			name:       "rate limit by message",
			err:        &codexStreamError{message: "You hit the rate limit, retry later"},
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonRateLimitExceeded,
			wantOK:     true,
		},
		{
			name:       "quota exhausted",
			err:        &codexStreamError{code: "insufficient_quota"},
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonQuotaExhausted,
			wantOK:     true,
		},
		{
			name:       "auth failure by code",
			err:        &codexStreamError{code: "invalid_api_key"},
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonAuthFailure,
			wantOK:     true,
		},
		{
			name:       "permission_denied by code",
			err:        &codexStreamError{code: "permission_denied"},
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonAuthFailure,
			wantOK:     true,
		},
		{
			name:       "billing hard limit by code",
			err:        &codexStreamError{code: "billing_hard_limit_reached"},
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonQuotaExhausted,
			wantOK:     true,
		},
		{
			name:   "unrecognized error stays unclassified",
			err:    &codexStreamError{code: "weird_thing", message: "something went wrong"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, reason, ok := classifyCodexStreamError(tt.err)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if scope != tt.wantScope {
				t.Errorf("scope = %q, want %q", scope, tt.wantScope)
			}
			if reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}
