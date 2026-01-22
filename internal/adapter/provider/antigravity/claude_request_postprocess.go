package antigravity

import (
	"encoding/json"
	"strings"
)

// antigravityIdentity is the identity instruction injected when user doesn't provide one
// (exactly like Antigravity-Manager's build_system_instruction)
const antigravityIdentity = `You are Antigravity, a powerful agentic AI coding assistant designed by the Google Deepmind team working on Advanced Agentic Coding.
You are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question.
**Absolute paths only**
**Proactiveness**`

// PostProcessClaudeRequest applies post-processing to the converted Gemini request
// Similar to CLIProxyAPI's request handling logic:
// 1. Injects Antigravity identity into system instruction (like Antigravity-Manager)
// 2. Cleans tool input schemas for Gemini compatibility (like Antigravity-Manager)
// 3. Injects interleaved thinking hint when tools + thinking are enabled
// 4. Closes broken tool loops by injecting synthetic messages (like Antigravity-Manager)
// 5. Uses cached signatures for thinking blocks
// 6. Applies skip_thought_signature_validator for tool calls without valid signatures
// 7. Merges adjacent messages with same role (like Antigravity-Manager)
// 8. Injects toolConfig, stopSequences, effortLevel (like Antigravity-Manager)
// 9. Validates signature model compatibility (like Antigravity-Manager)
//
// Note: cache_control cleaning is now done in adapter.go BEFORE transformation
func PostProcessClaudeRequest(geminiBody []byte, sessionID string, hasThinking bool, claudeRequest []byte, mappedModel string) []byte {
	var request map[string]interface{}
	if err := json.Unmarshal(geminiBody, &request); err != nil {
		return geminiBody
	}

	modified := injectAntigravityIdentity(request)

	// 1. Inject Antigravity identity into system instruction (like Antigravity-Manager)

	// 2. Clean tool input schemas for Gemini compatibility (like Antigravity-Manager)
	if cleanToolInputSchemas(request) {
		modified = true
	}

	// 3. Check if we have tools and thinking enabled
	hasTools := hasToolDeclarations(request)

	// 4. Inject interleaved thinking hint (like CLIProxyAPI)
	if hasThinking && hasTools {
		if injectInterleavedHint(request) {
			modified = true
		}
	}

	// 5. Close broken tool loops for thinking mode (like Antigravity-Manager)
	// This must be called when thinking is enabled to recover from stripped thinking blocks
	if hasThinking {
		if CloseToolLoopForThinking(request) {
			modified = true
		}
	}

	// 6. Merge adjacent messages with same role (like Antigravity-Manager)
	// Gemini API requires strict user/model role alternation
	if contents, ok := request["contents"].([]interface{}); ok {
		merged := MergeAdjacentRoles(contents)
		if len(merged) != len(contents) {
			request["contents"] = merged
			modified = true
		}
	}

	// 7. Process contents for signature caching, skip sentinel, and model compatibility
	if contents, ok := request["contents"].([]interface{}); ok {
		if processContentsForSignatures(contents, sessionID, mappedModel) {
			modified = true
		}
	}

	// 8. Inject toolConfig with VALIDATED mode when tools exist (like Antigravity-Manager)
	if InjectToolConfig(request) {
		modified = true
	}

	// 9. Inject stop sequences to generationConfig (like Antigravity-Manager)
	if InjectStopSequences(request) {
		modified = true
	}

	// 10. Inject effortLevel from Claude output_config.effort (like Antigravity-Manager)
	if claudeRequest != nil {
		if InjectEffortLevel(request, claudeRequest) {
			modified = true
		}
	}

	// 11. If thinking is disabled, clean all thinking-related fields recursively
	if !hasThinking {
		CleanThinkingFieldsRecursive(request)
		modified = true
	}

	if !modified {
		return geminiBody
	}

	result, err := json.Marshal(request)
	if err != nil {
		return geminiBody
	}
	return result
}

// checkForAntigravityIdentity checks if system instruction already contains Antigravity identity
// (like Antigravity-Manager's hybrid check)
func checkForAntigravityIdentity(sysInst map[string]interface{}) bool {
	parts, ok := sysInst["parts"].([]interface{})
	if !ok {
		return false
	}

	for _, part := range parts {
		if partMap, ok := part.(map[string]interface{}); ok {
			if text, ok := partMap["text"].(string); ok {
				if strings.Contains(text, "You are Antigravity") {
					return true
				}
			}
		}
	}
	return false
}

// injectAntigravityIdentity injects Antigravity identity into system instruction
// (exactly like Antigravity-Manager's build_system_instruction)
func injectAntigravityIdentity(request map[string]interface{}) bool {
	sysInst, ok := request["systemInstruction"].(map[string]interface{})
	if !ok {
		// No system instruction exists, create new one with identity
		request["systemInstruction"] = map[string]interface{}{
			"role": "user",
			"parts": []interface{}{
				map[string]interface{}{"text": antigravityIdentity},
				map[string]interface{}{"text": "\n--- [SYSTEM_PROMPT_END] ---"},
			},
		}
		return true
	}

	// Check if user already provided Antigravity identity
	if checkForAntigravityIdentity(sysInst) {
		// User already has Antigravity identity, don't inject
		return false
	}

	// Get existing parts
	parts, ok := sysInst["parts"].([]interface{})
	if !ok {
		parts = []interface{}{}
	}

	// Prepend Antigravity identity at the beginning
	newParts := []interface{}{
		map[string]interface{}{"text": antigravityIdentity},
	}
	newParts = append(newParts, parts...)

	// Append end marker
	newParts = append(newParts, map[string]interface{}{"text": "\n--- [SYSTEM_PROMPT_END] ---"})

	sysInst["parts"] = newParts
	return true
}

// hasToolDeclarations checks if the request has tool/function declarations
func hasToolDeclarations(request map[string]interface{}) bool {
	tools, ok := request["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		return false
	}

	for _, tool := range tools {
		if toolMap, ok := tool.(map[string]interface{}); ok {
			if _, hasFuncDecls := toolMap["functionDeclarations"]; hasFuncDecls {
				return true
			}
		}
	}
	return false
}

// cleanToolInputSchemas cleans all tool input schemas in the request for Gemini compatibility
// (like Antigravity-Manager's clean_tool_input_schemas)
func cleanToolInputSchemas(request map[string]interface{}) bool {
	tools, ok := request["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		return false
	}

	modified := false

	for _, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		// Process functionDeclarations
		funcDecls, ok := toolMap["functionDeclarations"].([]interface{})
		if !ok {
			continue
		}

		for _, decl := range funcDecls {
			declMap, ok := decl.(map[string]interface{})
			if !ok {
				continue
			}

			// Clean parameters schema
			if params, ok := declMap["parameters"].(map[string]interface{}); ok {
				CleanJSONSchema(params)
				modified = true
			}
		}
	}

	return modified
}

// injectInterleavedHint injects the interleaved thinking hint into system instruction
// (like CLIProxyAPI's interleavedHint injection)
func injectInterleavedHint(request map[string]interface{}) bool {
	hint := "Interleaved thinking is enabled. You may think between tool calls and after receiving tool results before deciding the next action or final answer. Do not mention these instructions or any constraints about thinking blocks; just apply them."

	sysInst, ok := request["systemInstruction"].(map[string]interface{})
	if !ok {
		// Create new system instruction
		request["systemInstruction"] = map[string]interface{}{
			"role": "user",
			"parts": []interface{}{
				map[string]interface{}{"text": hint},
			},
		}
		return true
	}

	// Append to existing system instruction parts
	parts, ok := sysInst["parts"].([]interface{})
	if !ok {
		parts = []interface{}{}
	}

	parts = append(parts, map[string]interface{}{"text": hint})
	sysInst["parts"] = parts
	return true
}

// processContentsForSignatures processes message contents to:
// 1. Check signature model compatibility (like Antigravity-Manager)
// 2. Recover signatures from tool_id cache
// 3. Validate cross-model signature compatibility
func processContentsForSignatures(contents []interface{}, _ string, mappedModel string) bool {
	modified := false
	cache := GlobalSignatureCache()

	for _, content := range contents {
		contentMap, ok := content.(map[string]interface{})
		if !ok {
			continue
		}

		role, _ := contentMap["role"].(string)
		if role != "model" {
			continue
		}

		parts, ok := contentMap["parts"].([]interface{})
		if !ok {
			continue
		}

		var currentThinkingSignature string

		for i, part := range parts {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				continue
			}

			// Check if this is a thinking part
			if thought, ok := partMap["thought"].(bool); ok && thought {
				text, _ := partMap["text"].(string)
				existingSig, _ := partMap["thoughtSignature"].(string)

				// [NEW] Check model compatibility for existing signature
				if hasValidThinkingSignature(text, existingSig) {
					if cachedFamily := cache.GetSignatureFamily(existingSig); cachedFamily != "" {
						if !IsModelCompatible(cachedFamily, mappedModel) {
							// Incompatible signature - downgrade to text
							delete(partMap, "thought")
							delete(partMap, "thoughtSignature")
							parts[i] = partMap
							modified = true
							continue
						}
					}
					currentThinkingSignature = existingSig
				} else {
					// Invalid or no signature - drop the thinking block
					// by removing it from parts (handled by the caller)
					currentThinkingSignature = ""
				}
			}

			// Check if this is a function call part
			if fc, hasFc := partMap["functionCall"].(map[string]interface{}); hasFc {
				// [NEW] Clean args to remove illegal JSON Schema fields (like Antigravity-Manager)
				// Some clients inject $schema, additionalProperties, etc. in tool arguments
				if args, ok := fc["args"].(map[string]interface{}); ok {
					CleanJSONSchema(args)
					fc["args"] = args
					partMap["functionCall"] = fc
					modified = true
				}

				existingSig, _ := partMap["thoughtSignature"].(string)

				// [FIX] Try to recover signature from tool_id cache (like Antigravity-Manager)
				if !HasValidSignature(existingSig) {
					if fcID, ok := fc["id"].(string); ok && fcID != "" {
						if cachedSig := cache.GetToolSignature(fcID); cachedSig != "" {
							// [NEW] Check model compatibility
							if cachedFamily := cache.GetSignatureFamily(cachedSig); cachedFamily != "" {
								if !IsModelCompatible(cachedFamily, mappedModel) {
									// Incompatible signature - skip
									continue
								}
							}
							existingSig = cachedSig
							partMap["thoughtSignature"] = cachedSig
							modified = true
						}
					}
				}

				// [CRITICAL FIX] Only add thoughtSignature if we have a valid one
				// Vertex AI v1internal rejects sentinel values like "skip_thought_signature_validator"
				// Unlike CLIProxyAPI, we must NOT use sentinel values as fallback
				if !HasValidSignature(existingSig) {
					if HasValidSignature(currentThinkingSignature) {
						partMap["thoughtSignature"] = currentThinkingSignature
						modified = true
					}
					// If no valid signature available, do NOT add the field
					// Vertex AI will handle this gracefully without the sentinel
				} else {
					// Ensure existing valid signature is set
					partMap["thoughtSignature"] = existingSig
					modified = true
				}
				parts[i] = partMap
			}
		}

		// Filter out unsigned thinking blocks
		var filteredParts []interface{}
		for _, part := range parts {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				filteredParts = append(filteredParts, part)
				continue
			}

			// Keep non-thinking parts
			thought, isThought := partMap["thought"].(bool)
			if !isThought || !thought {
				filteredParts = append(filteredParts, part)
				continue
			}

			// For thinking parts, only keep if they have valid signature
			text, _ := partMap["text"].(string)
			sig, _ := partMap["thoughtSignature"].(string)
			if hasValidThinkingSignature(text, sig) {
				filteredParts = append(filteredParts, part)
			}
			// Drop unsigned thinking blocks (they break API validation)
		}

		if len(filteredParts) != len(parts) {
			contentMap["parts"] = filteredParts
			modified = true
		}
	}

	return modified
}

// HasThinkingEnabled checks if thinking is enabled in the original request
func HasThinkingEnabled(requestBody []byte) bool {
	var request map[string]interface{}
	if err := json.Unmarshal(requestBody, &request); err != nil {
		return false
	}

	// Check for Claude format thinking config
	if thinking, ok := request["thinking"].(map[string]interface{}); ok {
		if thinkingType, _ := thinking["type"].(string); thinkingType == "enabled" {
			return true
		}
	}

	// Check for Gemini format thinking config
	if genConfig, ok := request["generationConfig"].(map[string]interface{}); ok {
		if thinkingConfig, ok := genConfig["thinkingConfig"].(map[string]interface{}); ok {
			if includeThoughts, _ := thinkingConfig["include_thoughts"].(bool); includeThoughts {
				return true
			}
		}
	}

	return false
}

// IsClaudeThinkingModel checks if the model supports thinking
func IsClaudeThinkingModel(model string) bool {
	modelLower := strings.ToLower(model)
	thinkingModels := []string{
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-3",
		"claude-sonnet-4",
		"claude-opus-4",
	}

	for _, m := range thinkingModels {
		if strings.Contains(modelLower, m) {
			return true
		}
	}

	return false
}

// ConversationState represents the state of conversation for tool loop detection
// (like Antigravity-Manager's ConversationState)
type ConversationState struct {
	InToolLoop       bool
	LastAssistantIdx int // -1 if not found
}

// analyzeConversationState analyzes the conversation to detect tool loops
// (like Antigravity-Manager's analyze_conversation_state)
func analyzeConversationState(contents []interface{}) ConversationState {
	state := ConversationState{
		LastAssistantIdx: -1,
	}

	if len(contents) == 0 {
		return state
	}

	// Find last model/assistant message index
	for i := len(contents) - 1; i >= 0; i-- {
		content, ok := contents[i].(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := content["role"].(string)
		if role == "model" {
			state.LastAssistantIdx = i
			break
		}
	}

	// Check if the very last message is a Tool Result (User role with functionResponse)
	lastContent, ok := contents[len(contents)-1].(map[string]interface{})
	if !ok {
		return state
	}

	role, _ := lastContent["role"].(string)
	if role != "user" {
		return state
	}

	parts, ok := lastContent["parts"].([]interface{})
	if !ok {
		return state
	}

	for _, part := range parts {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		// Check for functionResponse (tool_result in Gemini format)
		if _, hasFR := partMap["functionResponse"]; hasFR {
			state.InToolLoop = true
			break
		}
	}

	return state
}

// hasThinkingBlockInContent checks if a content has thinking blocks
func hasThinkingBlockInContent(content map[string]interface{}) bool {
	parts, ok := content["parts"].([]interface{})
	if !ok {
		return false
	}

	for _, part := range parts {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		// Check for thought: true (thinking block in Gemini format)
		if thought, ok := partMap["thought"].(bool); ok && thought {
			return true
		}
	}
	return false
}

// DefaultStopSequences are stop sequences added to generationConfig
// (like Antigravity-Manager's default stop sequences)
var DefaultStopSequences = []string{
	"<|user|>",
	"<|endoftext|>",
	"<|end_of_turn|>",
	"[DONE]",
	"\n\nHuman:",
}

// MapEffortLevel maps Claude effort level to Gemini effortLevel
// (like Antigravity-Manager's effort level mapping)
func MapEffortLevel(effort string) string {
	switch strings.ToLower(effort) {
	case "high":
		return "HIGH"
	case "medium":
		return "MEDIUM"
	case "low":
		return "LOW"
	default:
		return "HIGH" // Default to HIGH
	}
}

// CloseToolLoopForThinking recovers from broken tool loops by injecting synthetic messages
// (exactly like Antigravity-Manager's close_tool_loop_for_thinking)
//
// When client strips valid thinking blocks (leaving only ToolUse), and we are in a tool loop,
// the API will reject the request because "Assistant message must start with thinking".
// We cannot fake the signature.
// Solution: Close the loop artificially so the model starts fresh.
func CloseToolLoopForThinking(request map[string]interface{}) bool {
	contents, ok := request["contents"].([]interface{})
	if !ok || len(contents) == 0 {
		return false
	}

	state := analyzeConversationState(contents)

	if !state.InToolLoop {
		return false
	}

	// Check if the last assistant message has a thinking block
	hasThinking := false
	if state.LastAssistantIdx >= 0 && state.LastAssistantIdx < len(contents) {
		if content, ok := contents[state.LastAssistantIdx].(map[string]interface{}); ok {
			hasThinking = hasThinkingBlockInContent(content)
		}
	}

	// If we are in a tool loop BUT the assistant message has no thinking block,
	// we must break the loop by injecting synthetic messages
	if !hasThinking {
		// Strategy:
		// 1. Inject a "fake" Assistant message saying "Tool execution completed."
		// 2. Inject a "fake" User message saying "Proceed."
		// This forces the model to generate a NEW turn with a fresh Thinking block.

		syntheticAssistant := map[string]interface{}{
			"role": "model",
			"parts": []interface{}{
				map[string]interface{}{"text": "[Tool execution completed. Please proceed.]"},
			},
		}

		syntheticUser := map[string]interface{}{
			"role": "user",
			"parts": []interface{}{
				map[string]interface{}{"text": "Proceed."},
			},
		}

		contents = append(contents, syntheticAssistant, syntheticUser)
		request["contents"] = contents
		return true
	}

	return false
}

// MergeAdjacentRoles merges consecutive messages with the same role
// (like Antigravity-Manager's merge_adjacent_roles)
// Gemini API requires strict user/model role alternation
func MergeAdjacentRoles(contents []interface{}) []interface{} {
	if len(contents) == 0 {
		return contents
	}

	merged := make([]interface{}, 0, len(contents))
	currentMsg, ok := contents[0].(map[string]interface{})
	if !ok {
		return contents
	}

	for i := 1; i < len(contents); i++ {
		nextMsg, ok := contents[i].(map[string]interface{})
		if !ok {
			continue
		}

		currentRole, _ := currentMsg["role"].(string)
		nextRole, _ := nextMsg["role"].(string)

		if currentRole == nextRole {
			// Same role - merge parts
			currentParts, _ := currentMsg["parts"].([]interface{})
			nextParts, _ := nextMsg["parts"].([]interface{})
			if currentParts == nil {
				currentParts = []interface{}{}
			}
			if nextParts != nil {
				currentParts = append(currentParts, nextParts...)
			}
			currentMsg["parts"] = currentParts
		} else {
			// Different role - push current and start new
			merged = append(merged, currentMsg)
			currentMsg = nextMsg
		}
	}

	// Don't forget the last message
	merged = append(merged, currentMsg)
	return merged
}

// CleanThinkingFieldsRecursive recursively removes thought and thoughtSignature fields
// (like Antigravity-Manager's clean_thinking_fields_recursive)
// Used when thinking is disabled but request contains thinking-related fields
func CleanThinkingFieldsRecursive(val interface{}) {
	switch v := val.(type) {
	case map[string]interface{}:
		delete(v, "thought")
		delete(v, "thoughtSignature")
		for _, child := range v {
			CleanThinkingFieldsRecursive(child)
		}
	case []interface{}:
		for _, item := range v {
			CleanThinkingFieldsRecursive(item)
		}
	}
}

// InjectToolConfig adds toolConfig with functionCallingConfig.mode = "VALIDATED"
// (like Antigravity-Manager's tool config injection)
func InjectToolConfig(request map[string]interface{}) bool {
	tools, ok := request["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		return false
	}

	// Add toolConfig
	if _, exists := request["toolConfig"]; exists {
		return false
	}
	request["toolConfig"] = map[string]interface{}{
		"functionCallingConfig": map[string]interface{}{
			"mode": "VALIDATED",
		},
	}
	return true
}

// InjectStopSequences adds default stop sequences to generationConfig
// (like Antigravity-Manager's stop sequences injection)
func InjectStopSequences(request map[string]interface{}) bool {
	genConfig, ok := request["generationConfig"].(map[string]interface{})
	if !ok {
		genConfig = map[string]interface{}{}
		request["generationConfig"] = genConfig
	}

	// Only inject if not already present
	if _, exists := genConfig["stopSequences"]; exists {
		return false
	}

	genConfig["stopSequences"] = DefaultStopSequences
	return true
}

// InjectEffortLevel adds effortLevel to generationConfig from Claude output_config.effort
// (like Antigravity-Manager's effort level injection)
func InjectEffortLevel(request map[string]interface{}, claudeRequest []byte) bool {
	var claudeReq struct {
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
	}
	if err := json.Unmarshal(claudeRequest, &claudeReq); err != nil {
		return false
	}

	if claudeReq.OutputConfig.Effort == "" {
		return false
	}

	genConfig, ok := request["generationConfig"].(map[string]interface{})
	if !ok {
		genConfig = map[string]interface{}{}
		request["generationConfig"] = genConfig
	}

	genConfig["effortLevel"] = MapEffortLevel(claudeReq.OutputConfig.Effort)
	return true
}

// CleanCacheControlFromContents removes cache_control fields from message contents
// (like Antigravity-Manager's clean_cache_control_from_messages)
//
// VS Code and other clients may send back historical messages with cache_control
// which is not accepted by the API. This function deep cleans all cache_control fields.
func CleanCacheControlFromContents(contents []interface{}) bool {
	modified := false

	for _, content := range contents {
		contentMap, ok := content.(map[string]interface{})
		if !ok {
			continue
		}

		parts, ok := contentMap["parts"].([]interface{})
		if !ok {
			continue
		}

		for i, part := range parts {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				continue
			}

			// Remove cache_control from this part
			if _, hasCacheControl := partMap["cache_control"]; hasCacheControl {
				delete(partMap, "cache_control")
				parts[i] = partMap
				modified = true
			}

			// Also check nested structures (like inlineData, functionCall, etc.)
			for key, value := range partMap {
				if nestedMap, ok := value.(map[string]interface{}); ok {
					if _, hasCacheControl := nestedMap["cache_control"]; hasCacheControl {
						delete(nestedMap, "cache_control")
						partMap[key] = nestedMap
						parts[i] = partMap
						modified = true
					}
				}
			}
		}
	}

	return modified
}
