package service

import (
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/payloadoverride"
)

func validateSystemSettingValue(key, value string) error {
	switch key {
	case domain.SettingKeyPayloadOverrideRules:
		return payloadoverride.ValidateRulesJSON(value)
	default:
		return nil
	}
}
