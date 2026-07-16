package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/awsl-project/maxx/internal/codexguard"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/payloadoverride"
	"github.com/awsl-project/maxx/internal/reqpolicy"
)

func validateSystemSettingValue(key, value string) error {
	switch key {
	case domain.SettingKeyPayloadOverrideRules:
		return payloadoverride.ValidateRulesJSON(value)
	case domain.SettingKeyReasoningPolicy:
		return reqpolicy.ValidatePolicyJSON(value)
	case domain.SettingKeyCodexReasoningGuard:
		return validateCodexReasoningGuardSetting(value)
	case domain.SettingKeyRateLimitCooldownDefaultSeconds:
		return validateRateLimitCooldownDefaultSeconds(value)
	default:
		return nil
	}
}

func validateRateLimitCooldownDefaultSeconds(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%w: %s cannot be empty", domain.ErrInvalidInput, domain.SettingKeyRateLimitCooldownDefaultSeconds)
	}
	seconds, err := strconv.Atoi(trimmed)
	if err != nil || seconds < 1 || seconds > 86400 {
		return fmt.Errorf("%w: %s must be an integer between 1 and 86400", domain.ErrInvalidInput, domain.SettingKeyRateLimitCooldownDefaultSeconds)
	}
	return nil
}

func validateCodexReasoningGuardSetting(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s cannot be empty", domain.ErrInvalidInput, domain.SettingKeyCodexReasoningGuard)
	}
	if _, err := codexguard.ParseConfigJSON([]byte(value)); err != nil {
		return fmt.Errorf("%w: invalid %s: %v", domain.ErrInvalidInput, domain.SettingKeyCodexReasoningGuard, err)
	}
	return nil
}
