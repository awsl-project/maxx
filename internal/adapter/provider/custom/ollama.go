package custom

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/google/uuid"
)

const (
	customBackendOllama = "ollama"
)

var ollamaHTTPClient = &http.Client{Timeout: 10 * time.Minute}

type claudeMessageRequest struct {
	Model         string               `json:"model"`
	Messages      []claudeInputMessage `json:"messages"`
	System        json.RawMessage      `json:"system,omitempty"`
	Tools         []claudeInputTool    `json:"tools,omitempty"`
	ToolChoice    *claudeToolChoice    `json:"tool_choice,omitempty"`
	Thinking      *claudeThinking      `json:"thinking,omitempty"`
	MaxTokens     int                  `json:"max_tokens,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`
	StopSequences []string             `json:"stop_sequences,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
}

type claudeInputMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeInputTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type claudeToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type claudeThinking struct {
	Type string `json:"type"`
}

type ollamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []ollamaMessage        `json:"messages"`
	Tools    []ollamaTool           `json:"tools,omitempty"`
	Think    *bool                  `json:"think,omitempty"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	Thinking  string           `json:"thinking,omitempty"`
	Images    []string         `json:"images,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
	Error           string        `json:"error,omitempty"`
}

func (a *CustomAdapter) executeOllama(c *flow.Ctx, provider *domain.Provider) error {
	clientType := flow.GetClientType(c)
	if clientType != domain.ClientTypeClaude {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUnsupportedFormat, false, "Ollama custom backend only supports Claude-compatible requests")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	requestBody := flow.GetRequestBody(c)
	mappedModel := flow.GetMappedModel(c)
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}

	ollamaReq, claudeReq, err := buildOllamaChatRequest(requestBody, mappedModel)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, err.Error())
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	upstreamURL := buildUpstreamURL(a.getBaseURL(clientType), "/api/chat")
	payload, err := json.Marshal(ollamaReq)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to encode Ollama request")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(payload))
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "failed to create Ollama request")
		proxyErr.Scope = domain.ScopeEndpoint
		proxyErr.Reason = domain.CooldownReasonServerError
		return proxyErr
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "application/json")
	if apiKey := strings.TrimSpace(provider.Config.Custom.APIKey); apiKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  upstreamReq.Method,
			URL:     upstreamURL,
			Headers: sanitizeHeadersForEvent(upstreamReq.Header),
			Body:    string(payload),
		})
	}

	resp, err := ollamaHTTPClient.Do(upstreamReq)
	if err != nil {
		proxyErr := domain.NewScopedProxyError(domain.ErrUpstreamError, domain.ScopeProvider, domain.CooldownReasonNetworkError)
		proxyErr.Message = "failed to connect to Ollama"
		return proxyErr
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		if eventChan := flow.GetEventChan(c); eventChan != nil {
			eventChan.SendResponseInfo(&domain.ResponseInfo{Status: resp.StatusCode, Headers: flattenHeaders(resp.Header), Body: string(body)})
		}
		return classifyOllamaHTTPError(resp.StatusCode, body, ollamaReq.Model)
	}

	if ollamaReq.Stream {
		return a.handleOllamaStreamResponse(c, resp, claudeReq, ollamaReq.Model)
	}
	return a.handleOllamaNonStreamResponse(c, resp, claudeReq, ollamaReq.Model)
}

func buildOllamaChatRequest(body []byte, mappedModel string) (*ollamaChatRequest, *claudeMessageRequest, error) {
	var req claudeMessageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, nil, fmt.Errorf("invalid Claude request body: %w", err)
	}

	model := strings.TrimSpace(mappedModel)
	if model == "" {
		model = strings.TrimSpace(req.Model)
	}
	if model == "" {
		return nil, nil, errors.New("missing model for Ollama backend")
	}

	messages := make([]ollamaMessage, 0, len(req.Messages)+1)
	if systemText, err := claudeSystemToText(req.System); err != nil {
		return nil, nil, err
	} else if systemText != "" {
		messages = append(messages, ollamaMessage{Role: "system", Content: systemText})
	}

	toolNamesByID := map[string]string{}
	for _, msg := range req.Messages {
		converted, err := claudeMessageToOllamaMessages(msg, toolNamesByID)
		if err != nil {
			return nil, nil, err
		}
		messages = append(messages, converted...)
	}

	options := map[string]interface{}{}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}
	if req.Temperature != nil {
		options["temperature"] = *req.Temperature
	}
	if len(req.StopSequences) > 0 {
		options["stop"] = req.StopSequences
	}
	if len(options) == 0 {
		options = nil
	}

	return &ollamaChatRequest{
		Model:    model,
		Messages: messages,
		Tools:    convertClaudeToolsToOllama(req.Tools, req.ToolChoice),
		Think:    convertClaudeThinkingToOllama(req.Thinking),
		Stream:   req.Stream,
		Options:  options,
	}, &req, nil
}

func claudeSystemToText(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", nil
	}
	return claudeContentToText(raw)
}

func claudeMessageToOllamaMessages(msg claudeInputMessage, toolNamesByID map[string]string) ([]ollamaMessage, error) {
	role := strings.ToLower(strings.TrimSpace(msg.Role))
	if role != "user" && role != "assistant" && role != "system" {
		return nil, fmt.Errorf("unsupported Claude message role for Ollama backend: %s", msg.Role)
	}

	content, images, toolCalls, toolResults, thinking, err := claudeContentToOllamaParts(msg.Content, role, toolNamesByID)
	if err != nil {
		return nil, err
	}

	out := make([]ollamaMessage, 0, 1+len(toolResults))
	if content != "" || thinking != "" || len(images) > 0 || len(toolCalls) > 0 || len(toolResults) == 0 {
		out = append(out, ollamaMessage{Role: role, Content: content, Thinking: thinking, Images: images, ToolCalls: toolCalls})
	}
	out = append(out, toolResults...)
	return out, nil
}

type claudeToolResultPart struct {
	ToolUseID string
	Content   string
}

func claudeContentToOllamaParts(raw json.RawMessage, role string, toolNamesByID map[string]string) (string, []string, []ollamaToolCall, []ollamaMessage, string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", nil, nil, nil, "", nil
	}

	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		return s, nil, nil, nil, "", nil
	}

	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &blocks); err != nil {
		return "", nil, nil, nil, "", fmt.Errorf("unsupported Claude content shape for Ollama backend")
	}

	parts := make([]string, 0, len(blocks))
	images := []string{}
	toolCalls := []ollamaToolCall{}
	toolResults := []ollamaMessage{}
	thinkingParts := []string{}
	for _, block := range blocks {
		var blockType string
		_ = json.Unmarshal(block["type"], &blockType)
		switch blockType {
		case "text":
			var text string
			_ = json.Unmarshal(block["text"], &text)
			if strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		case "thinking":
			var text string
			_ = json.Unmarshal(block["thinking"], &text)
			if strings.TrimSpace(text) != "" {
				thinkingParts = append(thinkingParts, text)
			}
		case "redacted_thinking":
			// Anthropic redacted thinking cannot be replayed to Ollama. Drop it
			// rather than leaking an opaque provider-specific blob into content.
		case "image":
			image, err := claudeImageBlockToOllama(block)
			if err != nil {
				return "", nil, nil, nil, "", err
			}
			if image != "" {
				images = append(images, image)
			}
		case "tool_use":
			call, id, name, err := claudeToolUseBlockToOllama(block)
			if err != nil {
				return "", nil, nil, nil, "", err
			}
			if id != "" && name != "" {
				toolNamesByID[id] = name
			}
			toolCalls = append(toolCalls, call)
		case "tool_result":
			result, err := claudeToolResultBlockToText(block)
			if err != nil {
				return "", nil, nil, nil, "", err
			}
			toolName := toolNamesByID[result.ToolUseID]
			if toolName == "" {
				toolName = result.ToolUseID
			}
			toolResults = append(toolResults, ollamaMessage{Role: "tool", Content: result.Content, ToolName: toolName})
		case "document":
			return "", nil, nil, nil, "", fmt.Errorf("Ollama custom backend does not support Claude document content")
		default:
			if len(block) > 0 {
				parts = append(parts, string(mustJSON(block)))
			}
		}
	}
	return strings.Join(parts, "\n\n"), images, toolCalls, toolResults, strings.Join(thinkingParts, "\n\n"), nil
}

func claudeContentToText(raw json.RawMessage) (string, error) {
	content, _, _, toolResults, thinking, err := claudeContentToOllamaParts(raw, "", map[string]string{})
	if err != nil {
		return "", err
	}
	parts := []string{}
	if content != "" {
		parts = append(parts, content)
	}
	if thinking != "" {
		parts = append(parts, thinking)
	}
	for _, result := range toolResults {
		if result.ToolName != "" {
			parts = append(parts, "Tool result "+result.ToolName+":\n"+result.Content)
		} else {
			parts = append(parts, result.Content)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func claudeImageBlockToOllama(block map[string]json.RawMessage) (string, error) {
	var source map[string]json.RawMessage
	if err := json.Unmarshal(block["source"], &source); err != nil {
		return "", fmt.Errorf("invalid Claude image source for Ollama backend")
	}
	var sourceType string
	_ = json.Unmarshal(source["type"], &sourceType)
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case "base64":
		var data string
		_ = json.Unmarshal(source["data"], &data)
		if strings.TrimSpace(data) == "" {
			return "", fmt.Errorf("Claude image base64 source missing data for Ollama backend")
		}
		return data, nil
	case "url":
		return "", fmt.Errorf("Ollama custom backend does not support Claude image URL sources; use base64 image data")
	default:
		return "", fmt.Errorf("unsupported Claude image source type for Ollama backend: %s", sourceType)
	}
}

func claudeToolUseBlockToOllama(block map[string]json.RawMessage) (ollamaToolCall, string, string, error) {
	var id, name string
	_ = json.Unmarshal(block["id"], &id)
	_ = json.Unmarshal(block["name"], &name)
	if strings.TrimSpace(name) == "" {
		return ollamaToolCall{}, id, name, fmt.Errorf("Claude tool_use missing name for Ollama backend")
	}
	args := block["input"]
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage(`{}`)
	}
	return ollamaToolCall{Function: ollamaToolCallFunction{Name: name, Arguments: args}}, id, name, nil
}

func claudeToolResultBlockToText(block map[string]json.RawMessage) (claudeToolResultPart, error) {
	var toolUseID string
	_ = json.Unmarshal(block["tool_use_id"], &toolUseID)
	text, err := claudeContentToText(block["content"])
	if err != nil {
		var fallback string
		if json.Unmarshal(block["content"], &fallback) == nil {
			text = fallback
		} else {
			text = string(block["content"])
		}
	}
	return claudeToolResultPart{ToolUseID: toolUseID, Content: text}, nil
}

func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func convertClaudeToolsToOllama(tools []claudeInputTool, choice *claudeToolChoice) []ollamaTool {
	if len(tools) == 0 || (choice != nil && strings.EqualFold(strings.TrimSpace(choice.Type), "none")) {
		return nil
	}
	out := make([]ollamaTool, 0, len(tools))
	forceName := ""
	if choice != nil && strings.EqualFold(strings.TrimSpace(choice.Type), "tool") {
		forceName = strings.TrimSpace(choice.Name)
	}
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		if forceName != "" && tool.Name != forceName {
			continue
		}
		params := tool.InputSchema
		if len(bytes.TrimSpace(params)) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

func convertClaudeThinkingToOllama(thinking *claudeThinking) *bool {
	if thinking == nil {
		return nil
	}
	enabled := strings.EqualFold(strings.TrimSpace(thinking.Type), "enabled")
	return &enabled
}

func (a *CustomAdapter) handleOllamaNonStreamResponse(c *flow.Ctx, resp *http.Response, claudeReq *claudeMessageRequest, model string) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read Ollama response")
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonNetworkError
		return proxyErr
	}

	var ollamaResp ollamaChatResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "invalid Ollama response")
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonServerError
		return proxyErr
	}
	if ollamaResp.Error != "" {
		return classifyOllamaHTTPError(http.StatusBadGateway, []byte(ollamaResp.Error), model)
	}
	if ollamaResp.Model != "" {
		model = ollamaResp.Model
	}

	claudeResp := map[string]interface{}{
		"id":            "msg_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       ollamaMessageToClaudeContent(ollamaResp.Message),
		"stop_reason":   ollamaStopReasonToClaude(ollamaResp.DoneReason),
		"stop_sequence": nil,
		"usage": map[string]int{
			"input_tokens":  ollamaResp.PromptEvalCount,
			"output_tokens": ollamaResp.EvalCount,
		},
	}
	out, _ := json.Marshal(claudeResp)

	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendResponseInfo(&domain.ResponseInfo{Status: resp.StatusCode, Headers: flattenHeaders(resp.Header), Body: string(out)})
		eventChan.SendResponseModel(model)
		if ollamaResp.PromptEvalCount > 0 || ollamaResp.EvalCount > 0 {
			eventChan.SendMetrics(&domain.AdapterMetrics{InputTokens: uint64(ollamaResp.PromptEvalCount), OutputTokens: uint64(ollamaResp.EvalCount)})
		}
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(out)
	_ = claudeReq
	return nil
}

func (a *CustomAdapter) handleOllamaStreamResponse(c *flow.Ctx, resp *http.Response, claudeReq *claudeMessageRequest, model string) error {
	eventChan := flow.GetEventChan(c)
	if eventChan != nil {
		eventChan.SendResponseInfo(&domain.ResponseInfo{Status: resp.StatusCode, Headers: flattenHeaders(resp.Header), Body: "[streaming]"})
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	messageID := "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	sendSSE := func(event string, payload interface{}) error {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, b); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := sendSSE("message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]int{"input_tokens": 0, "output_tokens": 0},
		},
	}); err != nil {
		return clientDisconnectedErr(err)
	}

	writeSSEError := func(message string) error {
		return sendSSE("error", map[string]interface{}{
			"type": "error",
			"error": map[string]string{
				"type":    "upstream_error",
				"message": message,
			},
		})
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	inputTokens, outputTokens := 0, 0
	responseModel := model
	stopReason := "end_turn"
	firstTokenSent := false
	blockIndex := 0
	textBlockOpen := false
	contentOrToolSent := false

	startTextBlock := func() error {
		if textBlockOpen {
			return nil
		}
		if err := sendSSE("content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": blockIndex,
			"content_block": map[string]string{
				"type": "text",
				"text": "",
			},
		}); err != nil {
			return err
		}
		textBlockOpen = true
		contentOrToolSent = true
		return nil
	}
	stopTextBlock := func() error {
		if !textBlockOpen {
			return nil
		}
		if err := sendSSE("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": blockIndex}); err != nil {
			return err
		}
		textBlockOpen = false
		blockIndex++
		return nil
	}
	emitToolCall := func(call ollamaToolCall) error {
		if err := stopTextBlock(); err != nil {
			return err
		}
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			name = "tool"
		}
		toolID := "toolu_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if err := sendSSE("content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": blockIndex,
			"content_block": map[string]interface{}{
				"type":  "tool_use",
				"id":    toolID,
				"name":  name,
				"input": map[string]interface{}{},
			},
		}); err != nil {
			return err
		}
		partialJSON := "{}"
		if len(bytes.TrimSpace(call.Function.Arguments)) > 0 {
			partialJSON = string(call.Function.Arguments)
		}
		if err := sendSSE("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": blockIndex,
			"delta": map[string]string{"type": "input_json_delta", "partial_json": partialJSON},
		}); err != nil {
			return err
		}
		if err := sendSSE("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": blockIndex}); err != nil {
			return err
		}
		blockIndex++
		contentOrToolSent = true
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk ollamaChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			message := "invalid Ollama stream chunk: " + line
			if writeErr := writeSSEError(message); writeErr != nil {
				return clientDisconnectedErr(writeErr)
			}
			return classifyOllamaHTTPError(http.StatusBadGateway, []byte(line), model)
		}
		if chunk.Error != "" {
			if writeErr := writeSSEError(chunk.Error); writeErr != nil {
				return clientDisconnectedErr(writeErr)
			}
			return classifyOllamaHTTPError(http.StatusBadGateway, []byte(chunk.Error), model)
		}
		if chunk.Model != "" {
			responseModel = chunk.Model
		}
		if chunk.Message.Content != "" {
			if eventChan != nil && !firstTokenSent {
				eventChan.SendFirstToken(time.Now().UnixMilli())
				firstTokenSent = true
			}
			if err := startTextBlock(); err != nil {
				return clientDisconnectedErr(err)
			}
			if err := sendSSE("content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": blockIndex,
				"delta": map[string]string{"type": "text_delta", "text": chunk.Message.Content},
			}); err != nil {
				return clientDisconnectedErr(err)
			}
		}
		for _, call := range chunk.Message.ToolCalls {
			if eventChan != nil && !firstTokenSent {
				eventChan.SendFirstToken(time.Now().UnixMilli())
				firstTokenSent = true
			}
			if err := emitToolCall(call); err != nil {
				return clientDisconnectedErr(err)
			}
		}
		if chunk.PromptEvalCount > 0 {
			inputTokens = chunk.PromptEvalCount
		}
		if chunk.EvalCount > 0 {
			outputTokens = chunk.EvalCount
		}
		if chunk.DoneReason != "" {
			stopReason = ollamaStopReasonToClaude(chunk.DoneReason)
		}
	}
	if err := scanner.Err(); err != nil {
		message := "failed reading Ollama stream: " + err.Error()
		if writeErr := writeSSEError(message); writeErr != nil {
			return clientDisconnectedErr(writeErr)
		}
		proxyErr := domain.NewProxyErrorWithMessage(err, true, "failed reading Ollama stream")
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonNetworkError
		return proxyErr
	}

	if !contentOrToolSent {
		if err := startTextBlock(); err != nil {
			return clientDisconnectedErr(err)
		}
	}
	if err := stopTextBlock(); err != nil {
		return clientDisconnectedErr(err)
	}
	if err := sendSSE("message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{"output_tokens": outputTokens},
	}); err != nil {
		return clientDisconnectedErr(err)
	}
	if err := sendSSE("message_stop", map[string]string{"type": "message_stop"}); err != nil {
		return clientDisconnectedErr(err)
	}

	if eventChan != nil {
		eventChan.SendResponseModel(responseModel)
		if inputTokens > 0 || outputTokens > 0 {
			eventChan.SendMetrics(&domain.AdapterMetrics{InputTokens: uint64(inputTokens), OutputTokens: uint64(outputTokens)})
		}
	}
	_ = claudeReq
	return nil
}

func ollamaMessageToClaudeContent(message ollamaMessage) []map[string]interface{} {
	blocks := make([]map[string]interface{}, 0, 1+len(message.ToolCalls))
	if message.Content != "" || len(message.ToolCalls) == 0 {
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": message.Content})
	}
	for _, call := range message.ToolCalls {
		input := map[string]interface{}{}
		if len(call.Function.Arguments) > 0 {
			_ = json.Unmarshal(call.Function.Arguments, &input)
		}
		blocks = append(blocks, map[string]interface{}{
			"type":  "tool_use",
			"id":    "toolu_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			"name":  call.Function.Name,
			"input": input,
		})
	}
	return blocks
}

func ollamaStopReasonToClaude(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "stop", "", "done":
		return "end_turn"
	case "length":
		return "max_tokens"
	default:
		return "end_turn"
	}
}

func classifyOllamaHTTPError(status int, body []byte, model string) error {
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(status)
	}
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, status >= 500, "Ollama upstream error: "+msg)
	proxyErr.HTTPStatusCode = status
	proxyErr.Model = model
	switch status {
	case http.StatusNotFound:
		proxyErr.Scope = domain.ScopeModel
		proxyErr.Reason = domain.CooldownReasonModelUnavailable
		proxyErr.Retryable = false
	case http.StatusUnauthorized, http.StatusForbidden:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
	case http.StatusTooManyRequests:
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonRateLimitExceeded
		proxyErr.Retryable = true
	case http.StatusRequestEntityTooLarge, http.StatusBadRequest:
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Retryable = false
	default:
		if status >= 500 {
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Reason = domain.CooldownReasonServerError
		} else {
			proxyErr.Scope = domain.ScopeRequest
			proxyErr.Retryable = false
		}
	}
	return proxyErr
}

func clientDisconnectedErr(err error) error {
	proxyErr := domain.NewProxyErrorWithMessage(err, false, "client disconnected")
	proxyErr.Scope = domain.ScopeRequest
	return proxyErr
}
