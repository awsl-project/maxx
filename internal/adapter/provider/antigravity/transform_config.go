package antigravity

import (
	"log"
	"strings"
)

// buildGenerationConfig builds Gemini generationConfig from Claude request
// Reference: Antigravity-Manager's build_generation_config
func buildGenerationConfig(
	claudeReq *ClaudeRequest,
	mappedModel string,
	stream bool,
) map[string]interface{} {
	config := make(map[string]interface{})

	// 1. Thinking Configuration
	// Check if thinking is requested AND the target model supports it
	// Reference: Antigravity-Manager's target model support check
	thinkingRequested := claudeReq.Thinking != nil && claudeReq.Thinking.Type == "enabled"
	modelSupportsThinking := TargetModelSupportsThinking(mappedModel)

	if thinkingRequested && modelSupportsThinking {
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
	} else if thinkingRequested && !modelSupportsThinking {
		// Log when thinking is requested but model doesn't support it
		log.Printf("[Antigravity] Thinking requested but model %s doesn't support it, disabled", mappedModel)
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
	maxTokens := 64000 // Default
	if claudeReq.MaxTokens > 0 {
		maxTokens = claudeReq.MaxTokens
	}
	config["maxOutputTokens"] = maxTokens

	// 4. Stop Sequences
	config["stopSequences"] = DefaultStopSequences

	// 5. Effort Level (Output Config)
	if claudeReq.OutputConfig != nil && claudeReq.OutputConfig.Effort != "" {
		config["effortLevel"] = mapEffortLevel(claudeReq.OutputConfig.Effort)
	}

	// 6. Response MIME Type (always JSON for API compatibility)
	config["responseMimeType"] = "text/plain"

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
