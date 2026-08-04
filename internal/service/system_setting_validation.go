package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/reqpolicy"
)

func validateSystemSettingValue(key, value string) error {
	switch key {
	case domain.SettingKeyReasoningPolicy:
		return reqpolicy.ValidatePolicyJSON(value)
	case domain.SettingKeyForceRetryUpstreamErrors, domain.SettingKeyOpenAIChatStreamTimeoutsEnabled, domain.SettingKeyRequestFailureDetailsEnabled, domain.SettingKeyProxyRequestsDisabled, domain.SettingKeyUserPanelDailyCheckInEnabled, domain.SettingKeyInviteRegistrationAutoApproveEnabled:
		return validateBooleanSystemSetting(key, value)
	case domain.SettingKeyRateLimitCooldownDefaultSeconds:
		return validateRateLimitCooldownDefaultSeconds(value)
	case domain.SettingKeyUserPanelDailyCheckInAmount:
		return validateUserPanelDailyCheckInAmount(value)
	case domain.SettingKeyOpenAIChatStreamFirstEventTimeoutMS, domain.SettingKeyOpenAIChatStreamIdleTimeoutMS:
		return validateStreamTimeoutMilliseconds(key, value)
	default:
		return nil
	}
}

func validateUserPanelDailyCheckInAmount(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%w: %s cannot be empty", domain.ErrInvalidInput, domain.SettingKeyUserPanelDailyCheckInAmount)
	}
	amount, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || amount <= 0 || amount > 1000000 {
		return fmt.Errorf("%w: %s must be a positive number no greater than 1000000", domain.ErrInvalidInput, domain.SettingKeyUserPanelDailyCheckInAmount)
	}
	return nil
}

func validateBooleanSystemSetting(key, value string) error {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "true", "false":
		return nil
	default:
		return fmt.Errorf("%w: %s must be true or false", domain.ErrInvalidInput, key)
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

func validateStreamTimeoutMilliseconds(key, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%w: %s cannot be empty", domain.ErrInvalidInput, key)
	}
	milliseconds, err := strconv.Atoi(trimmed)
	if err != nil || milliseconds < 1000 || milliseconds > 600000 {
		return fmt.Errorf("%w: %s must be an integer between 1000 and 600000", domain.ErrInvalidInput, key)
	}
	return nil
}
