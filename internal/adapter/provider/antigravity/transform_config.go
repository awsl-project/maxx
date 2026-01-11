package antigravity

import (
	"strings"
)

// buildGenerationConfig builds Gemini generationConfig from Claude request
// Reference: Antigravity-Manager's build_generation_config
func buildGenerationConfig(
	claudeReq *ClaudeRequest,
	mappedModel string,
	stream bool,
	hasThinking bool, // Pre-calculated thinking state (after all checks)
) map[string]interface{} {
	config := make(map[string]interface{})

	// 1. Thinking Configuration
	// Use the pre-calculated hasThinking flag to avoid logic duplication
	// Reference: Antigravity-Manager's unified thinking resolution
	if hasThinking && claudeReq.Thinking != nil {
		thinkingConfig := map[string]interface{}{
			"includeThoughts": true,
		}

		if claudeReq.Thinking.BudgetTokens != nil {
			budget := *claudeReq.Thinking.BudgetTokens

			// Flash models and Web Search have a thinking budget limit of 24576
			// Reference: Antigravity-Manager's FLASH_THINKING_MAX_BUDGET
			if isFlashModel(mappedModel) || hasWebSearchTool(claudeReq) {
				if budget > 24576 {
					budget = 24576
				}
			}

			thinkingConfig["thinkingBudget"] = budget
		}

		config["thinkingConfig"] = thinkingConfig
	}

	// 2. Basic Parameters
	if claudeReq.Temperature != nil {
		config["temperature"] = *claudeReq.Temperature
	}
	if claudeReq.TopP != nil {
		config["topP"] = *claudeReq.TopP
	}
	if claudeReq.TopK != nil {
		config["topK"] = *claudeReq.TopK
	}

	// 3. Max Output Tokens
	// Manager uses fixed 64K cap for Claude compatibility
	config["maxOutputTokens"] = 64000

	// 4. Stop Sequences
	config["stopSequences"] = DefaultStopSequences

	// 5. Effort Level (Output Config)
	if claudeReq.OutputConfig != nil && claudeReq.OutputConfig.Effort != "" {
		config["effortLevel"] = mapEffortLevel(claudeReq.OutputConfig.Effort)
	}

	return config
}

// mapEffortLevel maps Claude effort level to Gemini effort level
func mapEffortLevel(effort string) string {
	switch strings.ToLower(effort) {
	case "high":
		return "HIGH"
	case "medium":
		return "MEDIUM"
	case "low":
		return "LOW"
	default:
		// Default to HIGH for best quality
		return "HIGH"
	}
}

// isFlashModel checks if the model is a Flash variant
func isFlashModel(modelName string) bool {
	modelLower := strings.ToLower(modelName)
	return strings.Contains(modelLower, "flash") ||
		strings.Contains(modelLower, "lite")
}
