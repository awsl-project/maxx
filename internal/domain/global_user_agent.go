package domain

import (
	"fmt"
	"strings"
)

const MaxGlobalUserAgentLength = 512

// NormalizeGlobalUserAgent trims and rejects values that cannot be safely used
// as an HTTP User-Agent header value.
func NormalizeGlobalUserAgent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > MaxGlobalUserAgentLength || strings.ContainsAny(trimmed, "\r\n") {
		return ""
	}
	return trimmed
}

// ValidateGlobalUserAgentSetting validates the persisted global User-Agent
// setting. Empty values are allowed so admins can clear the configured value
// while leaving the opt-in override switch off.
func ValidateGlobalUserAgentSetting(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > MaxGlobalUserAgentLength {
		return fmt.Errorf("%w: global_user_agent must be at most %d characters", ErrInvalidInput, MaxGlobalUserAgentLength)
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return fmt.Errorf("%w: global_user_agent cannot contain CR or LF", ErrInvalidInput)
	}
	return nil
}
