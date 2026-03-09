package converter

import (
	"encoding/json"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func init() {
	RegisterConverter(domain.ClientTypeCodex, domain.ClientTypeClaude, &codexToClaudeRequest{}, &codexToClaudeResponse{})
}

type codexToClaudeRequest struct{}
type codexToClaudeResponse struct{}

type claudeStreamState struct {
	HasToolCall bool
	BlockIndex  int
	ShortToOrig map[string]string
}

func (c *codexToClaudeRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	var req CodexRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	claudeReq := ClaudeRequest{
		Model:       model,
		Stream:      stream,
		MaxTokens:   req.MaxOutputTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	// Convert reasoning effort to Claude output_config
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		claudeReq.OutputConfig = &ClaudeOutputConfig{
			Effort: req.Reasoning.Effort,
		}
	}

	// Convert instructions to system prompt
	if req.Instructions != "" {
		claudeReq.System = req.Instructions
	}

	// Convert input to Claude messages
	switch input := req.Input.(type) {
	case string:
		claudeReq.Messages = append(claudeReq.Messages, ClaudeMessage{
			Role:    "user",
			Content: input,
		})
	case []interface{}:
		for _, item := range input {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			itemType, _ := m["type"].(string)
			role, _ := m["role"].(string)

			switch itemType {
			case "message":
				if role == "" {
					role = "user"
				}
				claudeReq.Messages = append(claudeReq.Messages, ClaudeMessage{
					Role:    role,
					Content: m["content"],
				})
			case "function_call":
				// Convert function call to tool_use block
				id, _ := m["id"].(string)
				if id == "" {
					id, _ = m["call_id"].(string)
				}
				name, _ := m["name"].(string)
				argStr, _ := m["arguments"].(string)
				var args interface{}
				json.Unmarshal([]byte(argStr), &args)
				claudeReq.Messages = append(claudeReq.Messages, ClaudeMessage{
					Role: "assistant",
					Content: []ClaudeContentBlock{{
						Type:  "tool_use",
						ID:    id,
						Name:  name,
						Input: args,
					}},
				})
			case "function_call_output":
				// Convert function call output to tool_result
				callID, _ := m["call_id"].(string)
				outputStr, _ := m["output"].(string)
				claudeReq.Messages = append(claudeReq.Messages, ClaudeMessage{
					Role: "user",
					Content: []ClaudeContentBlock{{
						Type:      "tool_result",
						ToolUseID: callID,
						Content:   outputStr,
					}},
				})
			}
		}
	}

	// Convert tools
	for _, tool := range req.Tools {
		claudeReq.Tools = append(claudeReq.Tools, ClaudeTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Parameters,
		})
	}

	return json.Marshal(claudeReq)
}

func (c *codexToClaudeResponse) Transform(body []byte) ([]byte, error) {
	return c.TransformWithState(body, nil)
}

func (c *codexToClaudeResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	st := getClaudeStreamState(state)
	var output []byte
	for _, event := range events {
		if event.Event == "done" {
			continue
		}

		root := gjson.ParseBytes(event.Data)
		if !root.Exists() {
			continue
		}

		eventType := root.Get("type").String()

		switch eventType {
		case "response.created":
			state.MessageID = root.Get("response.id").String()
			msgStart := map[string]interface{}{
				"type": "message_start",
				"message": map[string]interface{}{
					"id":    state.MessageID,
					"type":  "message",
					"role":  "assistant",
					"model": root.Get("response.model").String(),
					"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
				},
			}
			output = append(output, FormatSSE("message_start", msgStart)...)

		case "response.reasoning_summary_part.added":
			blockStart := map[string]interface{}{
				"type":  "content_block_start",
				"index": st.BlockIndex,
				"content_block": map[string]interface{}{
					"type":     "thinking",
					"thinking": "",
				},
			}
			output = append(output, FormatSSE("content_block_start", blockStart)...)

		case "response.reasoning_summary_text.delta":
			delta := root.Get("delta").String()
			claudeDelta := map[string]interface{}{
				"type":  "content_block_delta",
				"index": st.BlockIndex,
				"delta": map[string]interface{}{
					"type":     "thinking_delta",
					"thinking": delta,
				},
			}
			output = append(output, FormatSSE("content_block_delta", claudeDelta)...)

		case "response.reasoning_summary_part.done":
			blockStop := map[string]interface{}{
				"type":  "content_block_stop",
				"index": st.BlockIndex,
			}
			output = append(output, FormatSSE("content_block_stop", blockStop)...)
			st.BlockIndex++

		case "response.content_part.added":
			blockStart := map[string]interface{}{
				"type":  "content_block_start",
				"index": st.BlockIndex,
				"content_block": map[string]interface{}{
					"type": "text",
					"text": "",
				},
			}
			output = append(output, FormatSSE("content_block_start", blockStart)...)

		case "response.output_text.delta":
			delta := root.Get("delta").String()
			claudeDelta := map[string]interface{}{
				"type":  "content_block_delta",
				"index": st.BlockIndex,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": delta,
				},
			}
			output = append(output, FormatSSE("content_block_delta", claudeDelta)...)

		case "response.content_part.done":
			blockStop := map[string]interface{}{
				"type":  "content_block_stop",
				"index": st.BlockIndex,
			}
			output = append(output, FormatSSE("content_block_stop", blockStop)...)
			st.BlockIndex++

		case "response.output_item.added":
			item := root.Get("item")
			if item.Get("type").String() == "function_call" {
				st.HasToolCall = true
				if st.ShortToOrig == nil {
					st.ShortToOrig = buildReverseMapFromClaudeOriginalShortToOriginal(state.OriginalRequestBody)
				}
				name := item.Get("name").String()
				if orig, ok := st.ShortToOrig[name]; ok {
					name = orig
				}
				blockStart := map[string]interface{}{
					"type":  "content_block_start",
					"index": st.BlockIndex,
					"content_block": map[string]interface{}{
						"type": "tool_use",
						"id":   item.Get("call_id").String(),
						"name": name,
						"input": map[string]interface{}{},
					},
				}
				output = append(output, FormatSSE("content_block_start", blockStart)...)

				blockDelta := map[string]interface{}{
					"type":  "content_block_delta",
					"index": st.BlockIndex,
					"delta": map[string]interface{}{
						"type":         "input_json_delta",
						"partial_json": "",
					},
				}
				output = append(output, FormatSSE("content_block_delta", blockDelta)...)
			}

		case "response.function_call_arguments.delta":
			blockDelta := map[string]interface{}{
				"type":  "content_block_delta",
				"index": st.BlockIndex,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": root.Get("delta").String(),
				},
			}
			output = append(output, FormatSSE("content_block_delta", blockDelta)...)

		case "response.output_item.done":
			item := root.Get("item")
			if item.Get("type").String() == "function_call" {
				blockStop := map[string]interface{}{
					"type":  "content_block_stop",
					"index": st.BlockIndex,
				}
				output = append(output, FormatSSE("content_block_stop", blockStop)...)
				st.BlockIndex++
			}

		case "response.completed":
			stopReason := root.Get("response.stop_reason").String()
			if stopReason == "" {
				if st.HasToolCall {
					stopReason = "tool_use"
				} else {
					stopReason = "end_turn"
				}
			}
			inputTokens, outputTokens, cachedTokens := extractResponsesUsage(root.Get("response.usage"))
			msgDelta := map[string]interface{}{
				"type": "message_delta",
				"delta": map[string]interface{}{
					"stop_reason": stopReason,
				},
				"usage": map[string]int{
					"input_tokens":  inputTokens,
					"output_tokens": outputTokens,
				},
			}
			if cachedTokens > 0 {
				msgDelta["usage"].(map[string]int)["cache_read_input_tokens"] = cachedTokens
			}
			output = append(output, FormatSSE("message_delta", msgDelta)...)
			output = append(output, FormatSSE("message_stop", map[string]string{"type": "message_stop"})...)
		}
	}

	return output, nil
}

func getClaudeStreamState(state *TransformState) *claudeStreamState {
	if state.Custom == nil {
		state.Custom = &claudeStreamState{}
	}
	st, ok := state.Custom.(*claudeStreamState)
	if !ok || st == nil {
		st = &claudeStreamState{}
		state.Custom = st
	}
	return st
}

func (c *codexToClaudeResponse) TransformWithState(body []byte, state *TransformState) ([]byte, error) {
	root := gjson.ParseBytes(body)
	var response gjson.Result
	if root.Get("type").String() == "response.completed" && root.Get("response").Exists() {
		response = root.Get("response")
	} else if root.Get("output").Exists() {
		response = root
	} else {
		return nil, nil
	}

	revNames := map[string]string{}
	if state != nil && len(state.OriginalRequestBody) > 0 {
		revNames = buildReverseMapFromClaudeOriginalShortToOriginal(state.OriginalRequestBody)
	}

	out := `{"id":"","type":"message","role":"assistant","model":"","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}`
	out, _ = sjson.Set(out, "id", response.Get("id").String())
	out, _ = sjson.Set(out, "model", response.Get("model").String())
	inputTokens, outputTokens, cachedTokens := extractResponsesUsage(response.Get("usage"))
	out, _ = sjson.Set(out, "usage.input_tokens", inputTokens)
	out, _ = sjson.Set(out, "usage.output_tokens", outputTokens)
	if cachedTokens > 0 {
		out, _ = sjson.Set(out, "usage.cache_read_input_tokens", cachedTokens)
	}

	hasToolCall := false
	if output := response.Get("output"); output.Exists() && output.IsArray() {
		output.ForEach(func(_, item gjson.Result) bool {
			switch item.Get("type").String() {
			case "reasoning":
				thinkingBuilder := strings.Builder{}
				if summary := item.Get("summary"); summary.Exists() {
					if summary.IsArray() {
						summary.ForEach(func(_, part gjson.Result) bool {
							if txt := part.Get("text"); txt.Exists() {
								thinkingBuilder.WriteString(txt.String())
							} else {
								thinkingBuilder.WriteString(part.String())
							}
							return true
						})
					} else {
						thinkingBuilder.WriteString(summary.String())
					}
				}
				if thinkingBuilder.Len() == 0 {
					if content := item.Get("content"); content.Exists() {
						if content.IsArray() {
							content.ForEach(func(_, part gjson.Result) bool {
								if txt := part.Get("text"); txt.Exists() {
									thinkingBuilder.WriteString(txt.String())
								} else {
									thinkingBuilder.WriteString(part.String())
								}
								return true
							})
						} else {
							thinkingBuilder.WriteString(content.String())
						}
					}
				}
				if thinkingBuilder.Len() > 0 {
					block := `{"type":"thinking","thinking":""}`
					block, _ = sjson.Set(block, "thinking", thinkingBuilder.String())
					out, _ = sjson.SetRaw(out, "content.-1", block)
				}
			case "message":
				if content := item.Get("content"); content.Exists() {
					if content.IsArray() {
						content.ForEach(func(_, part gjson.Result) bool {
							if part.Get("type").String() == "output_text" {
								block := `{"type":"text","text":""}`
								block, _ = sjson.Set(block, "text", part.Get("text").String())
								out, _ = sjson.SetRaw(out, "content.-1", block)
							}
							return true
						})
					} else if content.Type == gjson.String {
						block := `{"type":"text","text":""}`
						block, _ = sjson.Set(block, "text", content.String())
						out, _ = sjson.SetRaw(out, "content.-1", block)
					}
				}
			case "function_call":
				hasToolCall = true
				callID := item.Get("call_id").String()
				name := item.Get("name").String()
				if orig, ok := revNames[name]; ok {
					name = orig
				}
				argsRaw := item.Get("arguments").String()
				var args interface{}
				if argsRaw != "" {
					_ = json.Unmarshal([]byte(argsRaw), &args)
				}
				block := `{"type":"tool_use","id":"","name":"","input":{}}`
				block, _ = sjson.Set(block, "id", callID)
				block, _ = sjson.Set(block, "name", name)
				if args != nil {
					block, _ = sjson.Set(block, "input", args)
				}
				out, _ = sjson.SetRaw(out, "content.-1", block)
			}
			return true
		})
	}

	stopReason := response.Get("stop_reason").String()
	if stopReason == "" {
		if hasToolCall {
			stopReason = "tool_use"
		} else {
			stopReason = "end_turn"
		}
	}
	out, _ = sjson.Set(out, "stop_reason", stopReason)

	return []byte(out), nil
}

func buildReverseMapFromClaudeOriginalShortToOriginal(original []byte) map[string]string {
	tools := gjson.GetBytes(original, "tools")
	rev := map[string]string{}
	if tools.IsArray() && len(tools.Array()) > 0 {
		var names []string
		arr := tools.Array()
		for i := 0; i < len(arr); i++ {
			t := arr[i]
			if t.Get("type").String() != "" {
				continue
			}
			if v := t.Get("name"); v.Exists() {
				names = append(names, v.String())
			}
		}
		if len(names) > 0 {
			m := buildShortNameMap(names)
			for orig, short := range m {
				rev[short] = orig
			}
		}
	}
	return rev
}

func extractResponsesUsage(usage gjson.Result) (int, int, int) {
	if !usage.Exists() {
		return 0, 0, 0
	}
	inputTokens := int(usage.Get("input_tokens").Int())
	outputTokens := int(usage.Get("output_tokens").Int())
	cachedTokens := int(usage.Get("input_tokens_details.cached_tokens").Int())
	return inputTokens, outputTokens, cachedTokens
}
