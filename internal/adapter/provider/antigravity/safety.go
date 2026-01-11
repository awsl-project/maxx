package antigravity

import (
	"os"
	"strings"
)

// SafetyThreshold represents a Gemini safety threshold level
type SafetyThreshold string

const (
	SafetyThresholdOff           SafetyThreshold = "OFF"
	SafetyThresholdBlockNone     SafetyThreshold = "BLOCK_NONE"
	SafetyThresholdBlockLowUp    SafetyThreshold = "BLOCK_LOW_AND_ABOVE"
	SafetyThresholdBlockMedUp    SafetyThreshold = "BLOCK_MEDIUM_AND_ABOVE"
	SafetyThresholdBlockHighOnly SafetyThreshold = "BLOCK_ONLY_HIGH"
)

// SafetyCategories are the Gemini safety categories
var SafetyCategories = []string{
	"HARM_CATEGORY_HARASSMENT",
	"HARM_CATEGORY_HATE_SPEECH",
	"HARM_CATEGORY_SEXUALLY_EXPLICIT",
	"HARM_CATEGORY_DANGEROUS_CONTENT",
	"HARM_CATEGORY_CIVIC_INTEGRITY",
}

// GeminiSafetySetting represents a single safety setting
type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GetSafetyThresholdFromEnv reads safety threshold from environment variable
// (like Antigravity-Manager's get_safety_threshold)
// Environment variable: GEMINI_SAFETY_THRESHOLD
// Valid values: OFF, BLOCK_NONE, BLOCK_LOW_AND_ABOVE, BLOCK_MEDIUM_AND_ABOVE, BLOCK_ONLY_HIGH
// Default: OFF (no filtering)
func GetSafetyThresholdFromEnv() SafetyThreshold {
	threshold := os.Getenv("GEMINI_SAFETY_THRESHOLD")
	if threshold == "" {
		return SafetyThresholdOff
	}

	switch strings.ToUpper(threshold) {
	case "OFF":
		return SafetyThresholdOff
	case "NONE":
		return SafetyThresholdBlockNone
	case "BLOCK_NONE":
		return SafetyThresholdBlockNone
	case "BLOCK_LOW_AND_ABOVE":
		return SafetyThresholdBlockLowUp
	case "BLOCK_MEDIUM_AND_ABOVE":
		return SafetyThresholdBlockMedUp
	case "BLOCK_ONLY_HIGH":
		return SafetyThresholdBlockHighOnly
	case "LOW":
		return SafetyThresholdBlockLowUp
	case "MEDIUM":
		return SafetyThresholdBlockMedUp
	case "HIGH":
		return SafetyThresholdBlockHighOnly
	default:
		return SafetyThresholdOff
	}
}

// BuildSafetySettings builds safety settings for all categories with the given threshold
// (like Antigravity-Manager's build_safety_settings)
func BuildSafetySettings(threshold SafetyThreshold) []GeminiSafetySetting {
	settings := make([]GeminiSafetySetting, 0, len(SafetyCategories))
	for _, category := range SafetyCategories {
		settings = append(settings, GeminiSafetySetting{
			Category:  category,
			Threshold: string(threshold),
		})
	}
	return settings
}

// BuildSafetySettingsMap builds safety settings as []map[string]interface{} for JSON marshaling
func BuildSafetySettingsMap(threshold SafetyThreshold) []map[string]interface{} {
	settings := make([]map[string]interface{}, 0, len(SafetyCategories))
	for _, category := range SafetyCategories {
		settings = append(settings, map[string]interface{}{
			"category":  category,
			"threshold": string(threshold),
		})
	}
	return settings
}
