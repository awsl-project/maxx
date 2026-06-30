package service

import (
	"fmt"
	"strings"

	"github.com/awsl-project/maxx/internal/codexguard"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/payloadoverride"
)

func validateSystemSettingValue(key, value string) error {
	switch key {
	case domain.SettingKeyPayloadOverrideRules:
		return payloadoverride.ValidateRulesJSON(value)
	case domain.SettingKeyCodexReasoningGuard:
		return validateCodexReasoningGuardSetting(value)
	default:
		return nil
	}
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
