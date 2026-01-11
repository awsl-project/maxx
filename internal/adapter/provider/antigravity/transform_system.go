package antigravity

import (
	"strings"
)

// buildSystemInstruction builds Gemini systemInstruction from Claude system prompt
// Reference: Antigravity-Manager's build_system_instruction
func buildSystemInstruction(claudeReq *ClaudeRequest, modelName string) map[string]interface{} {
	parts := []map[string]interface{}{}

	// 1. Check if user already provided Antigravity identity
	userHasAntigravity := false
	if claudeReq.System != nil {
		systemText := extractSystemText(claudeReq.System)
		if strings.Contains(strings.ToLower(systemText), "antigravity") {
			userHasAntigravity = true
		}
	}

	// 2. Inject Antigravity Identity (if user hasn't provided it)
	if !userHasAntigravity {
		parts = append(parts, map[string]interface{}{
			"text": AntigravityIdentity,
		})
	}

	// 3. Add user's system prompt
	if claudeReq.System != nil {
		switch sys := claudeReq.System.(type) {
		case string:
			if sys != "" {
				parts = append(parts, map[string]interface{}{
					"text": sys,
				})
			}
		case []interface{}:
			for _, block := range sys {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if text, ok := blockMap["text"].(string); ok && text != "" {
						parts = append(parts, map[string]interface{}{
							"text": text,
						})
					}
				}
			}
		}
	}

	// 4. Add end marker (if we injected Antigravity identity)
	// Reference: Antigravity-Manager line 488-491
	if !userHasAntigravity {
		parts = append(parts, map[string]interface{}{
			"text": "\n--- [SYSTEM_PROMPT_END] ---",
		})
	}

	if len(parts) == 0 {
		return nil
	}

	return map[string]interface{}{
		"role":  "user",
		"parts": parts,
	}
}

// AntigravityIdentity is the system identity injected into all requests
// Aligned with Antigravity-Manager's identity text (short form)
const AntigravityIdentity = `You are Antigravity, a powerful agentic AI coding assistant designed by the Google Deepmind team working on Advanced Agentic Coding.
You are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question.
**Absolute paths only**
**Proactiveness**`
