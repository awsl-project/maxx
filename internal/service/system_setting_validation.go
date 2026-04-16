package service

import (
	"fmt"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/payloadoverride"
)

func validateSystemSettingValue(key, value string) error {
	switch key {
	case domain.SettingKeyPayloadOverrideRules:
		return payloadoverride.ValidateRulesJSON(value)
	case domain.SettingKeyProxyRequestsDisabled:
		return validateBooleanSystemSetting(value)
	default:
		return nil
	}
}

func validateBooleanSystemSetting(value string) error {
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%w: boolean setting must not contain surrounding whitespace", domain.ErrInvalidInput)
	}

	switch value {
	case "true", "false":
		return nil
	default:
		return fmt.Errorf("%w: boolean setting must be \"true\" or \"false\"", domain.ErrInvalidInput)
	}
}
