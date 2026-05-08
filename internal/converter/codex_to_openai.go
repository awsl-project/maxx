package converter

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func init() {
	RegisterConverter(domain.ClientTypeCodex, domain.ClientTypeOpenAI, &codexToOpenAIRequest{}, &codexToOpenAIResponse{})
}

type codexToOpenAIRequest struct{}
type codexToOpenAIResponse struct{}

type openaiStreamState struct {
	Started     bool
	HasToolCall bool
	ToolCalls   map[int]*openaiToolCallState
	ShortToOrig map[string]string
	Index       int
	CreatedAt   int64
	Model       string
	FinishSent  bool
}

type openaiToolCallState struct {
	ID            string
	CallID        string
	Name          string
	NameSent      bool
	ArgsDeltaSent bool
}

func (c *codexToOpenAIRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	var req CodexRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	openaiReq := OpenAIRequest{
		Model:       model,
		Stream:      stream,
		MaxTokens:   req.MaxOutputTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		openaiReq.ReasoningEffort = req.Reasoning.Effort
	}
	if req.ServiceTier != "" {
		openaiReq.ServiceTier = req.ServiceTier
	}

	// Convert instructions to system message
	if req.Instructions != "" {
		openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
			Role:    "system",
			Content: req.Instructions,
		})
	}

	// Convert input to messages
	switch input := req.Input.(type) {
	case string:
		openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
			Role:    "user",
			Content: input,
		})
	case []interface{}:
		for _, item := range input {
			if m, ok := item.(map[string]interface{}); ok {
				itemType, _ := m["type"].(string)
				role, _ := m["role"].(string)
				switch itemType {
				case "message":
					if role == "" {
						role = "user"
					}
					openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
						Role:    role,
						Content: codexContentToOpenAI(m["content"]),
					})
				case "function_call":
					id, _ := m["id"].(string)
					if id == "" {
						id, _ = m["call_id"].(string)
					}
					name, _ := m["name"].(string)
					args, _ := m["arguments"].(string)
					openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
						Role: "assistant",
						ToolCalls: []OpenAIToolCall{{
							ID:   id,
							Type: "function",
							Function: OpenAIFunctionCall{
								Name:      name,
								Arguments: args,
							},
						}},
					})
				case "function_call_output":
					callID, _ := m["call_id"].(string)
					if callID == "" {
						continue
					}
					openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
						Role:       "tool",
						Content:    codexToolOutputToOpenAI(m["output"]),
						ToolCallID: callID,
					})
				}
			}
		}
	}

	// Convert tools
	for _, tool := range req.Tools {
		openaiReq.Tools = append(openaiReq.Tools, OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	return json.Marshal(openaiReq)
}

func codexContentToOpenAI(content interface{}) interface{} {
	switch value := content.(type) {
	case []interface{}:
		parts := make([]map[string]interface{}, 0, len(value))
		var textParts []string
		onlyText := true
		sawCodexPart := false
		for _, rawPart := range value {
			if text, ok := rawPart.(string); ok {
				if text == "" {
					continue
				}
				textParts = append(textParts, text)
				parts = append(parts, map[string]interface{}{
					"type": "text",
					"text": text,
				})
				continue
			}

			part, ok := rawPart.(map[string]interface{})
			if !ok {
				onlyText = false
				continue
			}
			switch part["type"] {
			case "input_text", "output_text", "text":
				sawCodexPart = true
				text, _ := part["text"].(string)
				if text == "" {
					continue
				}
				textParts = append(textParts, text)
				parts = append(parts, map[string]interface{}{
					"type": "text",
					"text": text,
				})
			case "input_image", "output_image", "image_url":
				sawCodexPart = true
				onlyText = false
				imageURL := codexImageURLToOpenAI(part["image_url"], part["detail"])
				if imageURL == nil {
					imageURL = codexImageURLToOpenAI(part["image"], part["detail"])
				}
				if imageURL == nil {
					imageURL = codexImageURLToOpenAI(part["url"], part["detail"])
				}
				if imageURL != nil {
					parts = append(parts, map[string]interface{}{
						"type":      "image_url",
						"image_url": imageURL,
					})
				}
			case "input_file", "file":
				sawCodexPart = true
				onlyText = false
				if file := codexFileToOpenAI(part); file != nil {
					parts = append(parts, map[string]interface{}{
						"type": "file",
						"file": file,
					})
				}
			default:
				onlyText = false
			}
		}
		if onlyText {
			return strings.Join(textParts, "")
		}
		if len(parts) > 0 {
			return parts
		}
		if sawCodexPart {
			return ""
		}
	}
	return content
}

func codexToolOutputToOpenAI(output interface{}) interface{} {
	if output == nil {
		return ""
	}
	if text, ok := output.(string); ok {
		return text
	}
	if _, ok := output.([]interface{}); ok {
		switch normalized := codexContentToOpenAI(output).(type) {
		case string:
			return normalized
		case []map[string]interface{}:
			return normalized
		}
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func codexImageURLToOpenAI(raw interface{}, detailRaw interface{}) map[string]interface{} {
	var imageURL map[string]interface{}
	switch image := raw.(type) {
	case string:
		if image == "" {
			return nil
		}
		imageURL = map[string]interface{}{"url": image}
	case map[string]interface{}:
		if _, ok := image["url"].(string); ok {
			imageURL = image
		}
	}
	if imageURL == nil {
		return nil
	}
	if detail, ok := detailRaw.(string); ok && detail != "" {
		if _, exists := imageURL["detail"]; !exists {
			imageURL["detail"] = detail
		}
	}
	return imageURL
}

func codexFileToOpenAI(part map[string]interface{}) map[string]interface{} {
	file := map[string]interface{}{}
	if fileID, ok := part["file_id"].(string); ok && fileID != "" {
		file["file_id"] = fileID
	} else if fileURL, ok := part["file_url"].(string); ok && fileURL != "" {
		file["file_id"] = fileURL
	}
	if fileData, ok := part["file_data"].(string); ok && fileData != "" {
		file["file_data"] = fileData
	}
	if filename, ok := part["filename"].(string); ok && filename != "" {
		file["filename"] = filename
	}
	if len(file) == 0 {
		return nil
	}
	return file
}

func (c *codexToOpenAIResponse) Transform(body []byte) ([]byte, error) {
	return c.TransformWithState(body, nil)
}

func (c *codexToOpenAIResponse) TransformWithState(body []byte, state *TransformState) ([]byte, error) {
	root := gjson.ParseBytes(body)
	var response gjson.Result
	if root.Get("type").String() == "response.completed" && root.Get("response").Exists() {
		response = root.Get("response")
	} else if root.Get("output").Exists() {
		response = root
	} else {
		return body, nil
	}

	template := `{"id":"","object":"chat.completion","created":123456,"model":"model","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":null,"tool_calls":null},"finish_reason":null,"native_finish_reason":null}]}`

	if modelResult := response.Get("model"); modelResult.Exists() {
		template, _ = sjson.Set(template, "model", modelResult.String())
	}
	if createdAtResult := response.Get("created_at"); createdAtResult.Exists() {
		template, _ = sjson.Set(template, "created", createdAtResult.Int())
	} else {
		template, _ = sjson.Set(template, "created", time.Now().Unix())
	}
	if idResult := response.Get("id"); idResult.Exists() {
		template, _ = sjson.Set(template, "id", idResult.String())
	}

	if usageResult := response.Get("usage"); usageResult.Exists() {
		template = applyOpenAIUsage(template, usageResult)
	}

	outputResult := response.Get("output")
	if outputResult.IsArray() {
		var contentParts []string
		var reasoningParts []string
		var toolCalls []string
		rev := buildReverseMapFromOriginalOpenAI(nil)
		if state != nil && len(state.OriginalRequestBody) > 0 {
			rev = buildReverseMapFromOriginalOpenAI(state.OriginalRequestBody)
		}

		outputResult.ForEach(func(_, outputItem gjson.Result) bool {
			switch outputItem.Get("type").String() {
			case "reasoning":
				if summaryResult := outputItem.Get("summary"); summaryResult.IsArray() {
					summaryResult.ForEach(func(_, summaryItem gjson.Result) bool {
						if summaryItem.Get("type").String() == "summary_text" {
							if text := summaryItem.Get("text").String(); text != "" {
								reasoningParts = append(reasoningParts, text)
							}
						}
						return true
					})
				}
			case "message":
				if contentResult := outputItem.Get("content"); contentResult.IsArray() {
					contentResult.ForEach(func(_, contentItem gjson.Result) bool {
						switch contentItem.Get("type").String() {
						case "output_text":
							if text := contentItem.Get("text").String(); text != "" {
								contentParts = append(contentParts, text)
							}
						case "refusal":
							if text := contentItem.Get("refusal").String(); text != "" {
								contentParts = append(contentParts, text)
							}
						}
						return true
					})
				}
			case "function_call":
				functionCallTemplate := `{"id":"","type":"function","function":{"name":"","arguments":""}}`
				if callIDResult := outputItem.Get("call_id"); callIDResult.Exists() {
					functionCallTemplate, _ = sjson.Set(functionCallTemplate, "id", callIDResult.String())
				}
				if nameResult := outputItem.Get("name"); nameResult.Exists() {
					name := nameResult.String()
					if orig, ok := rev[name]; ok {
						name = orig
					}
					functionCallTemplate, _ = sjson.Set(functionCallTemplate, "function.name", name)
				}
				if argsResult := outputItem.Get("arguments"); argsResult.Exists() {
					functionCallTemplate, _ = sjson.Set(functionCallTemplate, "function.arguments", argsResult.String())
				}
				toolCalls = append(toolCalls, functionCallTemplate)
			}
			return true
		})

		if len(contentParts) > 0 {
			template, _ = sjson.Set(template, "choices.0.message.content", strings.Join(contentParts, ""))
			template, _ = sjson.Set(template, "choices.0.message.role", "assistant")
		}
		if len(reasoningParts) > 0 {
			template, _ = sjson.Set(template, "choices.0.message.reasoning_content", strings.Join(reasoningParts, ""))
			template, _ = sjson.Set(template, "choices.0.message.role", "assistant")
		}
		if len(toolCalls) > 0 {
			template, _ = sjson.SetRaw(template, "choices.0.message.tool_calls", `[]`)
			for _, toolCall := range toolCalls {
				template, _ = sjson.SetRaw(template, "choices.0.message.tool_calls.-1", toolCall)
			}
			template, _ = sjson.Set(template, "choices.0.message.role", "assistant")
		}
	}

	if statusResult := response.Get("status"); statusResult.Exists() {
		finishReason := ""
		switch statusResult.String() {
		case "completed":
			finishReason = "stop"
			if strings.Contains(template, `"tool_calls":[`) {
				finishReason = "tool_calls"
			}
		case "incomplete":
			finishReason = codexIncompleteReasonToOpenAI(response.Get("incomplete_details.reason").String())
		}
		if finishReason != "" {
			template, _ = sjson.Set(template, "choices.0.finish_reason", finishReason)
			template, _ = sjson.Set(template, "choices.0.native_finish_reason", finishReason)
		}
	}

	return []byte(template), nil
}

func (c *codexToOpenAIResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	st := getOpenAIStreamState(state)
	var output []byte
	for _, event := range events {
		if event.Event == "done" {
			if !st.FinishSent {
				output = append(output, buildOpenAIStreamDone(state.MessageID, st.HasToolCall)...)
				st.FinishSent = true
			}
			output = append(output, FormatDone()...)
			continue
		}

		raw := bytes.TrimSpace(event.Data)
		if len(raw) == 0 {
			continue
		}
		root := gjson.ParseBytes(raw)
		if !root.Exists() {
			continue
		}

		eventType := root.Get("type").String()

		switch eventType {
		case "response.created":
			state.MessageID = root.Get("response.id").String()
			st.CreatedAt = root.Get("response.created_at").Int()
			st.Model = root.Get("response.model").String()

		case "response.reasoning_summary_text.delta":
			if delta := root.Get("delta"); delta.Exists() {
				chunk := newOpenAIStreamTemplate(state.MessageID, st)
				chunk, _ = sjson.Set(chunk, "choices.0.delta.role", "assistant")
				chunk, _ = sjson.Set(chunk, "choices.0.delta.reasoning_content", delta.String())
				chunk = applyOpenAIUsageFromResponse(chunk, root.Get("response.usage"))
				output = append(output, FormatSSE("", []byte(chunk))...)
			}

		case "response.reasoning_summary_text.done":
			chunk := newOpenAIStreamTemplate(state.MessageID, st)
			chunk, _ = sjson.Set(chunk, "choices.0.delta.role", "assistant")
			chunk, _ = sjson.Set(chunk, "choices.0.delta.reasoning_content", "\n\n")
			chunk = applyOpenAIUsageFromResponse(chunk, root.Get("response.usage"))
			output = append(output, FormatSSE("", []byte(chunk))...)

		case "response.output_text.delta":
			if delta := root.Get("delta"); delta.Exists() {
				chunk := newOpenAIStreamTemplate(state.MessageID, st)
				chunk, _ = sjson.Set(chunk, "choices.0.delta.role", "assistant")
				chunk, _ = sjson.Set(chunk, "choices.0.delta.content", delta.String())
				chunk = applyOpenAIUsageFromResponse(chunk, root.Get("response.usage"))
				output = append(output, FormatSSE("", []byte(chunk))...)
			}

		case "response.output_item.added":
			item := root.Get("item")
			if item.Exists() && item.Get("type").String() == "function_call" {
				index := openAIStreamToolIndex(root, st)
				st.HasToolCall = true

				id := item.Get("call_id").String()
				if id == "" {
					id = item.Get("id").String()
				}
				name := openAIStreamToolName(item.Get("name").String(), state, st)

				toolState := st.ToolCalls[index]
				if toolState == nil {
					toolState = &openaiToolCallState{}
					st.ToolCalls[index] = toolState
				}
				toolState.ID = id
				toolState.CallID = item.Get("call_id").String()
				toolState.Name = name
				toolState.NameSent = true

				functionCallItemTemplate := `{"index":0,"id":"","type":"function","function":{"name":"","arguments":""}}`
				functionCallItemTemplate, _ = sjson.Set(functionCallItemTemplate, "index", index)
				functionCallItemTemplate, _ = sjson.Set(functionCallItemTemplate, "id", id)
				functionCallItemTemplate, _ = sjson.Set(functionCallItemTemplate, "function.name", name)

				chunk := newOpenAIStreamTemplate(state.MessageID, st)
				chunk, _ = sjson.Set(chunk, "choices.0.delta.role", "assistant")
				chunk, _ = sjson.SetRaw(chunk, "choices.0.delta.tool_calls", `[]`)
				chunk, _ = sjson.SetRaw(chunk, "choices.0.delta.tool_calls.-1", functionCallItemTemplate)
				chunk = applyOpenAIUsageFromResponse(chunk, root.Get("response.usage"))
				output = append(output, FormatSSE("", []byte(chunk))...)
			}

		case "response.function_call_arguments.delta":
			if delta := root.Get("delta"); delta.Exists() {
				index := openAIStreamToolIndex(root, st)
				st.HasToolCall = true
				toolState := st.ToolCalls[index]
				if toolState == nil {
					toolState = &openaiToolCallState{}
					st.ToolCalls[index] = toolState
				}
				toolState.ArgsDeltaSent = true

				functionCallItemTemplate := `{"index":0,"function":{"arguments":""}}`
				functionCallItemTemplate, _ = sjson.Set(functionCallItemTemplate, "index", index)
				functionCallItemTemplate, _ = sjson.Set(functionCallItemTemplate, "function.arguments", delta.String())

				chunk := newOpenAIStreamTemplate(state.MessageID, st)
				chunk, _ = sjson.Set(chunk, "choices.0.delta.role", "assistant")
				chunk, _ = sjson.SetRaw(chunk, "choices.0.delta.tool_calls", `[]`)
				chunk, _ = sjson.SetRaw(chunk, "choices.0.delta.tool_calls.-1", functionCallItemTemplate)
				chunk = applyOpenAIUsageFromResponse(chunk, root.Get("response.usage"))
				output = append(output, FormatSSE("", []byte(chunk))...)
			}

		case "response.output_item.done":
			item := root.Get("item")
			if item.Exists() && item.Get("type").String() == "function_call" {
				index := openAIStreamToolIndex(root, st)
				if toolState := st.ToolCalls[index]; toolState != nil && toolState.ArgsDeltaSent {
					continue
				}
				st.HasToolCall = true
				functionCallItemTemplate := `{"index":0,"id":"","type":"function","function":{"name":"","arguments":""}}`
				functionCallItemTemplate, _ = sjson.Set(functionCallItemTemplate, "index", index)
				functionCallItemTemplate, _ = sjson.Set(functionCallItemTemplate, "id", item.Get("call_id").String())

				name := openAIStreamToolName(item.Get("name").String(), state, st)
				functionCallItemTemplate, _ = sjson.Set(functionCallItemTemplate, "function.name", name)
				functionCallItemTemplate, _ = sjson.Set(functionCallItemTemplate, "function.arguments", item.Get("arguments").String())

				chunk := newOpenAIStreamTemplate(state.MessageID, st)
				chunk, _ = sjson.Set(chunk, "choices.0.delta.role", "assistant")
				chunk, _ = sjson.SetRaw(chunk, "choices.0.delta.tool_calls", `[]`)
				chunk, _ = sjson.SetRaw(chunk, "choices.0.delta.tool_calls.-1", functionCallItemTemplate)
				chunk = applyOpenAIUsageFromResponse(chunk, root.Get("response.usage"))
				output = append(output, FormatSSE("", []byte(chunk))...)
			}

		case "response.completed":
			if !st.FinishSent {
				chunk := newOpenAIStreamTemplate(state.MessageID, st)
				finishReason := "stop"
				if st.HasToolCall {
					finishReason = "tool_calls"
				}
				chunk, _ = sjson.Set(chunk, "choices.0.finish_reason", finishReason)
				chunk, _ = sjson.Set(chunk, "choices.0.native_finish_reason", finishReason)
				chunk = applyOpenAIUsageFromResponse(chunk, root.Get("response.usage"))
				output = append(output, FormatSSE("", []byte(chunk))...)
				st.FinishSent = true
			}
		}
	}

	return output, nil
}

func codexIncompleteReasonToOpenAI(reason string) string {
	switch reason {
	case "max_output_tokens", "max_tokens":
		return "length"
	case "content_filter":
		return "content_filter"
	default:
		return "stop"
	}
}

func openAIStreamToolIndex(root gjson.Result, st *openaiStreamState) int {
	if index := root.Get("output_index"); index.Exists() {
		return int(index.Int())
	}
	st.Index++
	return st.Index
}

func openAIStreamToolName(name string, state *TransformState, st *openaiStreamState) string {
	rev := st.ShortToOrig
	if rev == nil {
		rev = buildReverseMapFromOriginalOpenAI(state.OriginalRequestBody)
		st.ShortToOrig = rev
	}
	if orig, ok := rev[name]; ok {
		return orig
	}
	return name
}

func getOpenAIStreamState(state *TransformState) *openaiStreamState {
	if state.Custom == nil {
		state.Custom = &openaiStreamState{
			ToolCalls: map[int]*openaiToolCallState{},
			Index:     -1,
		}
	}
	st, ok := state.Custom.(*openaiStreamState)
	if !ok || st == nil {
		st = &openaiStreamState{
			ToolCalls: map[int]*openaiToolCallState{},
			Index:     -1,
		}
		state.Custom = st
	}
	return st
}

func buildOpenAIStreamDone(id string, hasToolCalls bool) []byte {
	finishReason := "stop"
	if hasToolCalls {
		finishReason = "tool_calls"
	}
	openaiChunk := OpenAIStreamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Choices: []OpenAIChoice{{
			Index:        0,
			Delta:        &OpenAIMessage{},
			FinishReason: finishReason,
		}},
	}
	return FormatSSE("", openaiChunk)
}

func newOpenAIStreamTemplate(id string, st *openaiStreamState) string {
	template := `{"id":"","object":"chat.completion.chunk","created":12345,"model":"","choices":[{"index":0,"delta":{"role":null,"content":null,"reasoning_content":null,"tool_calls":null},"finish_reason":null,"native_finish_reason":null}]}`
	template, _ = sjson.Set(template, "id", id)
	if st != nil && st.CreatedAt > 0 {
		template, _ = sjson.Set(template, "created", st.CreatedAt)
	} else {
		template, _ = sjson.Set(template, "created", time.Now().Unix())
	}
	if st != nil && st.Model != "" {
		template, _ = sjson.Set(template, "model", st.Model)
	}
	return template
}

func buildReverseMapFromOriginalOpenAI(original []byte) map[string]string {
	tools := gjson.GetBytes(original, "tools")
	rev := map[string]string{}
	if tools.IsArray() && len(tools.Array()) > 0 {
		var names []string
		arr := tools.Array()
		for i := 0; i < len(arr); i++ {
			t := arr[i]
			if t.Get("type").String() != "function" {
				continue
			}
			fn := t.Get("function")
			if !fn.Exists() {
				continue
			}
			if v := fn.Get("name"); v.Exists() {
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

func applyOpenAIUsageFromResponse(template string, usage gjson.Result) string {
	if !usage.Exists() {
		return template
	}
	return applyOpenAIUsage(template, usage)
}

func applyOpenAIUsage(template string, usage gjson.Result) string {
	if outputTokensResult := usage.Get("output_tokens"); outputTokensResult.Exists() {
		template, _ = sjson.Set(template, "usage.completion_tokens", outputTokensResult.Int())
	}
	if totalTokensResult := usage.Get("total_tokens"); totalTokensResult.Exists() {
		template, _ = sjson.Set(template, "usage.total_tokens", totalTokensResult.Int())
	}
	if inputTokensResult := usage.Get("input_tokens"); inputTokensResult.Exists() {
		template, _ = sjson.Set(template, "usage.prompt_tokens", inputTokensResult.Int())
	}
	if reasoningTokensResult := usage.Get("output_tokens_details.reasoning_tokens"); reasoningTokensResult.Exists() {
		template, _ = sjson.Set(template, "usage.completion_tokens_details.reasoning_tokens", reasoningTokensResult.Int())
	}
	return template
}
