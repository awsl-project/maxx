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
	customBackendOllama      = "ollama"
	defaultOllamaNumCtx      = 32768
	maxOllamaStreamLineBytes = 64 * 1024 * 1024
)

var ollamaHTTPClient = &http.Client{Timeout: 10 * time.Minute}

type claudeMessageRequest struct {
	Model         string               `json:"model"`
	Messages      []claudeInputMessage `json:"messages"`
	System        json.RawMessage      `json:"system,omitempty"`
	Tools         []claudeInputTool    `json:"tools,omitempty"`
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

type ollamaChatRequest struct {
	Model     string                 `json:"model"`
	Messages  []ollamaMessage        `json:"messages"`
	Tools     []ollamaTool           `json:"tools,omitempty"`
	Stream    bool                   `json:"stream"`
	Options   map[string]interface{} `json:"options,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
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

type ollamaOptionsConfig struct {
	NumCtx    int
	KeepAlive string
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

	ollamaReq, claudeReq, err := buildOllamaChatRequest(requestBody, mappedModel, resolveOllamaOptions(provider))
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
			Headers: flattenHeaders(upstreamReq.Header),
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

func resolveOllamaOptions(provider *domain.Provider) ollamaOptionsConfig {
	options := ollamaOptionsConfig{NumCtx: defaultOllamaNumCtx}
	if provider == nil || provider.Config == nil || provider.Config.Custom == nil || provider.Config.Custom.Ollama == nil {
		return options
	}
	if provider.Config.Custom.Ollama.NumCtx > 0 {
		options.NumCtx = provider.Config.Custom.Ollama.NumCtx
	}
	options.KeepAlive = strings.TrimSpace(provider.Config.Custom.Ollama.KeepAlive)
	return options
}

func buildOllamaChatRequest(body []byte, mappedModel string, ollamaOptions ollamaOptionsConfig) (*ollamaChatRequest, *claudeMessageRequest, error) {
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

	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" && role != "system" {
			return nil, nil, fmt.Errorf("unsupported Claude message role for Ollama backend: %s", msg.Role)
		}
		text, err := claudeContentToText(msg.Content)
		if err != nil {
			return nil, nil, err
		}
		messages = append(messages, ollamaMessage{Role: role, Content: text})
	}

	options := map[string]interface{}{}
	if ollamaOptions.NumCtx > 0 {
		options["num_ctx"] = ollamaOptions.NumCtx
	}
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
		Model:     model,
		Messages:  messages,
		Tools:     convertClaudeToolsToOllama(req.Tools),
		Stream:    req.Stream,
		Options:   options,
		KeepAlive: ollamaOptions.KeepAlive,
	}, &req, nil
}

func claudeSystemToText(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", nil
	}
	return claudeContentToText(raw)
}

func claudeContentToText(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", nil
	}

	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		return s, nil
	}

	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &blocks); err != nil {
		return "", fmt.Errorf("unsupported Claude content shape for Ollama backend")
	}

	parts := make([]string, 0, len(blocks))
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
		case "tool_result":
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
			if toolUseID != "" {
				parts = append(parts, "Tool result "+toolUseID+":\n"+text)
			} else {
				parts = append(parts, text)
			}
		case "tool_use":
			// Preserve assistant-side tool call history as text so Ollama keeps context
			// without pretending it has Claude's native content-block protocol.
			parts = append(parts, string(mustJSON(block)))
		case "image", "document":
			return "", fmt.Errorf("Ollama custom backend does not support Claude %s content", blockType)
		default:
			if len(block) > 0 {
				parts = append(parts, string(mustJSON(block)))
			}
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func convertClaudeToolsToOllama(tools []claudeInputTool) []ollamaTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ollamaTool, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
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
	if err := sendSSE("content_block_start", map[string]interface{}{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]string{
			"type": "text",
			"text": "",
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

	lineReader := newOllamaLineReader(resp.Body)
	inputTokens, outputTokens := 0, 0
	responseModel := model
	stopReason := "end_turn"
	firstTokenSent := false
	for {
		line, readErr := lineReader.readLine()
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			message := "failed reading Ollama stream: " + readErr.Error()
			if writeErr := writeSSEError(message); writeErr != nil {
				return clientDisconnectedErr(writeErr)
			}
			proxyErr := domain.NewProxyErrorWithMessage(readErr, true, "failed reading Ollama stream")
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Reason = domain.CooldownReasonNetworkError
			return proxyErr
		}
		line = strings.TrimSpace(line)
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
			if err := sendSSE("content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]string{"type": "text_delta", "text": chunk.Message.Content},
			}); err != nil {
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

	if err := sendSSE("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": 0}); err != nil {
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

type ollamaLineReader struct {
	r *bufio.Reader
}

func newOllamaLineReader(r io.Reader) *ollamaLineReader {
	return &ollamaLineReader{r: bufio.NewReaderSize(r, 64*1024)}
}

func (r *ollamaLineReader) readLine() (string, error) {
	var out strings.Builder
	for {
		part, err := r.r.ReadString('\n')
		if out.Len()+len(part) > maxOllamaStreamLineBytes {
			return "", fmt.Errorf("Ollama stream line exceeds %d bytes", maxOllamaStreamLineBytes)
		}
		out.WriteString(part)
		if err != nil {
			if errors.Is(err, io.EOF) && out.Len() > 0 {
				return out.String(), nil
			}
			return "", err
		}
		if strings.HasSuffix(part, "\n") {
			return out.String(), nil
		}
	}
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
	msg := extractOllamaErrorMessage(body)
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

func extractOllamaErrorMessage(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	var parsed struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(trimmed, &parsed) == nil {
		if strings.TrimSpace(parsed.Error) != "" {
			return strings.TrimSpace(parsed.Error)
		}
		if strings.TrimSpace(parsed.Message) != "" {
			return strings.TrimSpace(parsed.Message)
		}
	}
	return strings.TrimSpace(string(trimmed))
}

func clientDisconnectedErr(err error) error {
	proxyErr := domain.NewProxyErrorWithMessage(err, false, "client disconnected")
	proxyErr.Scope = domain.ScopeRequest
	return proxyErr
}
