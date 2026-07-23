package requestmeta

import (
	"strings"
	"unicode"

	"github.com/tidwall/gjson"
)

// ReasoningEffort extracts the client-requested reasoning depth from the
// request formats accepted by MAXX. It intentionally reads the original
// client payload so converted requests keep the user's requested depth.
func ReasoningEffort(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	for _, path := range []string{
		"reasoning.effort",                              // OpenAI Responses / Codex
		"reasoning_effort",                              // OpenAI Chat Completions
		"output_config.effort",                          // Claude adaptive thinking
		"generationConfig.thinkingConfig.thinkingLevel", // Gemini
		"generation_config.thinking_config.thinking_level",
	} {
		value := gjson.GetBytes(body, path)
		if value.Type != gjson.String {
			continue
		}
		if effort := normalizeEffort(value.String()); effort != "" {
			return effort
		}
	}

	thinkingType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String()))
	switch thinkingType {
	case "adaptive":
		return "auto"
	case "disabled":
		return "none"
	case "enabled":
		budget := gjson.GetBytes(body, "thinking.budget_tokens")
		if !budget.Exists() {
			return "auto"
		}
		if budget.Type != gjson.Number {
			return ""
		}
		return effortForThinkingBudget(budget.Int())
	default:
		return ""
	}
}

func effortForThinkingBudget(budget int64) string {
	switch {
	case budget < -1:
		return ""
	case budget == -1:
		return "auto"
	case budget <= 0:
		return "none"
	case budget <= 1024:
		return "low"
	case budget <= 8192:
		return "medium"
	default:
		return "high"
	}
}

func normalizeEffort(effort string) string {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" || len(effort) > 32 {
		return ""
	}
	for _, r := range effort {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return ""
		}
	}
	return effort
}
