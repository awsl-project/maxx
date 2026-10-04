package service

import (
	"encoding/json"
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
	case domain.SettingKeyForceRetryUpstreamErrors, domain.SettingKeyOpenAIChatStreamTimeoutsEnabled, domain.SettingKeyGlobalUserAgentOverrideEnabled, domain.SettingKeyRequestFailureDetailsEnabled, domain.SettingKeyStrictSupportModelsRoutingEnabled, domain.SettingKeyProxyRequestsDisabled, domain.SettingKeyUserPanelDailyCheckInEnabled, domain.SettingKeyUserPanelErrorChargeEnabled, domain.SettingKeyExternalModelListEnabled, domain.SettingKeyInviteRegistrationAutoApproveEnabled, domain.SettingKeyProxyManagementEnabled:
		return validateBooleanSystemSetting(key, value)
	case domain.SettingKeyGlobalUserAgent:
		return domain.ValidateGlobalUserAgentSetting(value)
	case domain.SettingKeyRateLimitCooldownDefaultSeconds, domain.SettingKeyOpenAIAPIKeyCooldownSeconds:
		return validateRateLimitCooldownDefaultSeconds(value)
	case domain.SettingKeyUserPanelDailyCheckInAmount:
		return validateUserPanelDailyCheckInAmount(value)
	case domain.SettingKeyUserPanelDailyCheckInBlacklistUserIDs:
		return validateUserPanelDailyCheckInBlacklistUserIDs(value)
	case domain.SettingKeyUserPanelErrorChargeCodes:
		return validateUserPanelErrorChargeCodes(value)
	case domain.SettingKeyUserPanelErrorChargeThreshold:
		return validateUserPanelErrorChargeThreshold(value)
	case domain.SettingKeyUserPanelErrorChargeAmount:
		return validateUserPanelErrorChargeAmount(value)
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

func validateUserPanelDailyCheckInBlacklistUserIDs(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	var ids []uint64
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return fmt.Errorf("%w: %s must be a JSON array of user IDs", domain.ErrInvalidInput, domain.SettingKeyUserPanelDailyCheckInBlacklistUserIDs)
	}
	for _, id := range ids {
		if id == 0 {
			return fmt.Errorf("%w: %s cannot contain zero user ID", domain.ErrInvalidInput, domain.SettingKeyUserPanelDailyCheckInBlacklistUserIDs)
		}
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
		return fmt.Errorf("%w: cooldown seconds cannot be empty", domain.ErrInvalidInput)
	}
	seconds, err := strconv.Atoi(trimmed)
	if err != nil || seconds < 1 || seconds > 604800 {
		return fmt.Errorf("%w: cooldown seconds must be an integer between 1 and 604800", domain.ErrInvalidInput)
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

func validateUserPanelErrorChargeCodes(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	for _, field := range strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		code, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || code < 100 || code > 599 {
			return fmt.Errorf("%w: %s must contain HTTP status codes between 100 and 599", domain.ErrInvalidInput, domain.SettingKeyUserPanelErrorChargeCodes)
		}
	}
	return nil
}

func validateUserPanelErrorChargeThreshold(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%w: %s cannot be empty", domain.ErrInvalidInput, domain.SettingKeyUserPanelErrorChargeThreshold)
	}
	threshold, err := strconv.Atoi(trimmed)
	if err != nil || threshold < 1 || threshold > 100 {
		return fmt.Errorf("%w: %s must be an integer between 1 and 100", domain.ErrInvalidInput, domain.SettingKeyUserPanelErrorChargeThreshold)
	}
	return nil
}

func validateUserPanelErrorChargeAmount(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%w: %s cannot be empty", domain.ErrInvalidInput, domain.SettingKeyUserPanelErrorChargeAmount)
	}
	amount, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || amount <= 0 || amount > 1000000 {
		return fmt.Errorf("%w: %s must be a positive number no greater than 1000000", domain.ErrInvalidInput, domain.SettingKeyUserPanelErrorChargeAmount)
	}
	return nil
}
