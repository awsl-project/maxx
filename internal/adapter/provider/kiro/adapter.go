package kiro

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	ctxutil "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/usage"
)

func init() {
	provider.RegisterAdapterFactory("kiro", NewAdapter)
}

// TokenCache caches access tokens
type TokenCache struct {
	AccessToken string
	ExpiresAt   time.Time
}

// UsageCache caches usage limits (only updated on manual refresh)
type UsageCache struct {
	UsageLimits *UsageLimits
	CachedAt    time.Time
}

// KiroAdapter handles communication with AWS CodeWhisperer/Q Developer
type KiroAdapter struct {
	provider   *domain.Provider
	tokenCache *TokenCache
	tokenMu    sync.RWMutex
	usageCache *UsageCache
	usageMu    sync.RWMutex
	httpClient *http.Client
}

// NewAdapter creates a new Kiro adapter
func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Kiro == nil {
		return nil, fmt.Errorf("provider %s missing kiro config", p.Name)
	}
	return &KiroAdapter{
		provider:   p,
		tokenCache: &TokenCache{},
		usageCache: &UsageCache{},
		httpClient: newKiroHTTPClient(),
	}, nil
}

// SupportedClientTypes returns the list of client types this adapter natively supports
func (a *KiroAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude}
}

// Execute performs the proxy request to the upstream CodeWhisperer API
func (a *KiroAdapter) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request, provider *domain.Provider) error {
	requestModel := ctxutil.GetRequestModel(ctx)
	requestBody := ctxutil.GetRequestBody(ctx)
	stream := ctxutil.GetIsStream(ctx)

	config := provider.Config.Kiro

	// Get region (default to us-east-1)
	region := config.Region
	if region == "" {
		region = DefaultRegion
	}

	// Get access token
	accessToken, err := a.getAccessToken(ctx)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, "failed to get access token")
	}

	// Convert Claude request to CodeWhisperer format (传入 req 用于生成稳定会话ID)
	cwBody, mappedModel, err := ConvertClaudeToCodeWhisperer(requestBody, config.ModelMapping, req)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, fmt.Sprintf("failed to convert request: %v", err))
	}

	// Update attempt record with the mapped model (kiro-specific internal mapping)
	if attempt := ctxutil.GetUpstreamAttempt(ctx); attempt != nil {
		attempt.MappedModel = mappedModel
	}

	// Get EventChannel for sending events to executor
	eventChan := ctxutil.GetEventChan(ctx)

	// Build upstream URL
	upstreamURL := fmt.Sprintf(CodeWhispererURLTemplate, region)

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(cwBody))
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, "failed to create upstream request")
	}

	// Set headers (matching kiro2api/server/common.go:168-177)
	upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
	upstreamReq.Header.Set("Content-Type", "application/json")
	if stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	}
	// 添加上游请求必需的header (硬编码匹配 kiro2api)
	upstreamReq.Header.Set("x-amzn-kiro-agent-mode", "spec")
	upstreamReq.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.18 KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1")
	upstreamReq.Header.Set("user-agent", "aws-sdk-js/1.0.18 ua/2.1 os/darwin#25.0.0 lang/js md/nodejs#20.16.0 api/codewhispererstreaming#1.0.18 m/E KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1")

	// Send request info via EventChannel
	eventChan.SendRequestInfo(&domain.RequestInfo{
		Method:  upstreamReq.Method,
		URL:     upstreamURL,
		Headers: flattenHeaders(upstreamReq.Header),
		Body:    string(cwBody),
	})

	// Execute request
	resp, err := a.httpClient.Do(upstreamReq)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream")
		proxyErr.IsNetworkError = true
		return proxyErr
	}
	defer resp.Body.Close()

	// Check for 401 (token expired) and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		// Invalidate token cache
		a.tokenMu.Lock()
		a.tokenCache = &TokenCache{}
		a.tokenMu.Unlock()

		// Get new token
		accessToken, err = a.getAccessToken(ctx)
		if err != nil {
			return domain.NewProxyErrorWithMessage(err, true, "failed to refresh access token")
		}

		// Retry request (matching kiro2api headers)
		upstreamReq, _ = http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(cwBody))
		upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
		upstreamReq.Header.Set("Content-Type", "application/json")
		if stream {
			upstreamReq.Header.Set("Accept", "text/event-stream")
		}
		upstreamReq.Header.Set("x-amzn-kiro-agent-mode", "spec")
		upstreamReq.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.18 KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1")
		upstreamReq.Header.Set("user-agent", "aws-sdk-js/1.0.18 ua/2.1 os/darwin#25.0.0 lang/js md/nodejs#20.16.0 api/codewhispererstreaming#1.0.18 m/E KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1")

		resp, err = a.httpClient.Do(upstreamReq)
		if err != nil {
			proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream after token refresh")
			proxyErr.IsNetworkError = true
			return proxyErr
		}
		defer resp.Body.Close()
	}

	// Check for error response
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)

		// Send error response info via EventChannel
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    string(body),
		})

		proxyErr := domain.NewProxyErrorWithMessage(
			fmt.Errorf("upstream error: %s", string(body)),
			isRetryableStatusCode(resp.StatusCode),
			fmt.Sprintf("upstream returned status %d", resp.StatusCode),
		)
		proxyErr.HTTPStatusCode = resp.StatusCode
		proxyErr.IsServerError = resp.StatusCode >= 500 && resp.StatusCode < 600

		return proxyErr
	}

	// Handle response (CodeWhisperer always returns streaming EventStream)
	// Calculate input tokens for the request
	inputTokens := calculateInputTokens(requestBody)

	if stream {
		return a.handleStreamResponse(ctx, w, resp, requestModel, inputTokens)
	}
	return a.handleCollectedStreamResponse(ctx, w, resp, requestModel, inputTokens)
}

// getAccessToken gets a valid access token, refreshing if necessary
func (a *KiroAdapter) getAccessToken(ctx context.Context) (string, error) {
	// Check cache
	a.tokenMu.RLock()
	if a.tokenCache.AccessToken != "" && time.Now().Before(a.tokenCache.ExpiresAt) {
		token := a.tokenCache.AccessToken
		a.tokenMu.RUnlock()
		return token, nil
	}
	a.tokenMu.RUnlock()

	// Refresh token
	config := a.provider.Config.Kiro
	tokenInfo, err := a.refreshToken(ctx, config)
	if err != nil {
		return "", err
	}

	// Cache token
	a.tokenMu.Lock()
	a.tokenCache = &TokenCache{
		AccessToken: tokenInfo.AccessToken,
		ExpiresAt:   time.Now().Add(time.Duration(tokenInfo.ExpiresIn-60) * time.Second), // 60s buffer
	}
	a.tokenMu.Unlock()

	return tokenInfo.AccessToken, nil
}

// refreshToken refreshes the access token based on auth method
func (a *KiroAdapter) refreshToken(ctx context.Context, config *domain.ProviderConfigKiro) (*RefreshResponse, error) {
	switch config.AuthMethod {
	case "social":
		return a.refreshSocialToken(ctx, config.RefreshToken)
	case "idc":
		return a.refreshIdCToken(ctx, config)
	default:
		return nil, fmt.Errorf("unsupported auth method: %s", config.AuthMethod)
	}
}

// refreshSocialToken refreshes token using Social authentication
// 匹配 kiro2api/auth/refresh.go:27-69
func (a *KiroAdapter) refreshSocialToken(ctx context.Context, refreshToken string) (*RefreshResponse, error) {
	reqBody, err := FastMarshal(RefreshRequest{RefreshToken: refreshToken})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", RefreshTokenURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 使用共享 HTTP 客户端 (匹配 kiro2api)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("refresh failed: status %d, response: %s", resp.StatusCode, string(body))
	}

	var result RefreshResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if err := FastUnmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// refreshIdCToken refreshes token using IdC (Identity Center) authentication
// 匹配 kiro2api/auth/refresh.go:72-131
func (a *KiroAdapter) refreshIdCToken(ctx context.Context, config *domain.ProviderConfigKiro) (*RefreshResponse, error) {
	reqBody, err := FastMarshal(IdcRefreshRequest{
		ClientId:     config.ClientID,
		ClientSecret: config.ClientSecret,
		GrantType:    "refresh_token",
		RefreshToken: config.RefreshToken,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal IdC request: %w", err)
	}

	// 使用硬编码 URL (匹配 kiro2api/config/config.go:22)
	req, err := http.NewRequestWithContext(ctx, "POST", IdcRefreshTokenURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create IdC request: %w", err)
	}

	// Set IdC specific headers (匹配 kiro2api/auth/refresh.go:92-100)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", "oidc.us-east-1.amazonaws.com")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/3.738.0 ua/2.1 os/other lang/js md/browser#unknown_unknown api/sso-oidc#3.738.0 m/E KiroIDE")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "*")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("User-Agent", "node")
	req.Header.Set("Accept-Encoding", "br, gzip, deflate")

	// 使用共享 HTTP 客户端 (匹配 kiro2api)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IdC request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IdC refresh failed: status %d, response: %s", resp.StatusCode, string(body))
	}

	var result RefreshResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read IdC response: %w", err)
	}
	if err := FastUnmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode IdC response: %w", err)
	}

	return &result, nil
}

// handleStreamResponse handles streaming EventStream response
func (a *KiroAdapter) handleStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, requestModel string, inputTokens int) error {
	eventChan := ctxutil.GetEventChan(ctx)

	// Send initial response info
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    "[streaming]",
	})

	// Set streaming headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Capture SSE output for attempt record
	var sseBuffer strings.Builder
	tee := &teeWriter{primary: w, buffer: &sseBuffer}

	streamCtx, err := newStreamProcessorContext(w, requestModel, inputTokens, tee)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
	}

	if err := streamCtx.sendInitialEvents(); err != nil {
		inTok, outTok := streamCtx.GetTokenCounts()
		a.sendFinalEvents(ctx, sseBuffer.String(), inTok, outTok, requestModel, streamCtx.GetFirstTokenTimeMs())
		return domain.NewProxyErrorWithMessage(err, false, "failed to send initial events")
	}

	err = streamCtx.processEventStream(ctx, resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			inTok, outTok := streamCtx.GetTokenCounts()
			a.sendFinalEvents(ctx, sseBuffer.String(), inTok, outTok, requestModel, streamCtx.GetFirstTokenTimeMs())
			return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
		}

		_ = streamCtx.sendFinalEvents()
		inTok, outTok := streamCtx.GetTokenCounts()
		a.sendFinalEvents(ctx, sseBuffer.String(), inTok, outTok, requestModel, streamCtx.GetFirstTokenTimeMs())
		return nil
	}

	if err := streamCtx.sendFinalEvents(); err != nil {
		inTok, outTok := streamCtx.GetTokenCounts()
		a.sendFinalEvents(ctx, sseBuffer.String(), inTok, outTok, requestModel, streamCtx.GetFirstTokenTimeMs())
		return domain.NewProxyErrorWithMessage(err, false, "failed to send final events")
	}

	inTok, outTok := streamCtx.GetTokenCounts()
	a.sendFinalEvents(ctx, sseBuffer.String(), inTok, outTok, requestModel, streamCtx.GetFirstTokenTimeMs())
	return nil
}

// sendFinalEvents sends final events via EventChannel
func (a *KiroAdapter) sendFinalEvents(ctx context.Context, body string, inputTokens, outputTokens int, requestModel string, firstTokenTimeMs int64) {
	eventChan := ctxutil.GetEventChan(ctx)
	if eventChan == nil {
		return
	}

	// Send first token time if available (for TTFT tracking)
	if firstTokenTimeMs > 0 {
		eventChan.SendFirstToken(firstTokenTimeMs)
	}

	// Send response info with body
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  200, // streaming always returns 200 at this point
		Body:    body,
	})

	// Try to extract usage metrics from the SSE content first
	if metrics := usage.ExtractFromStreamContent(body); metrics != nil && !metrics.IsEmpty() {
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:          metrics.InputTokens,
			OutputTokens:         metrics.OutputTokens,
			CacheReadCount:       metrics.CacheReadCount,
			CacheCreationCount:   metrics.CacheCreationCount,
			Cache5mCreationCount: metrics.Cache5mCreationCount,
			Cache1hCreationCount: metrics.Cache1hCreationCount,
		})
	} else {
		// Fall back to estimated token counts
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:  uint64(inputTokens),
			OutputTokens: uint64(outputTokens),
		})
	}

	// Set responseModel to requestModel since Kiro doesn't return model in response
	eventChan.SendResponseModel(requestModel)
}

// handleCollectedStreamResponse collects streaming response into a single JSON response
func (a *KiroAdapter) handleCollectedStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, requestModel string, inputTokens int) error {
	eventChan := ctxutil.GetEventChan(ctx)

	// Send initial response info
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    "[stream-collected]",
	})

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream stream")
	}

	parser := NewCompliantEventStreamParser()
	result, parseErr := parser.ParseResponse(body)
	if parseErr != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, false, "failed to parse upstream stream")
	}

	// Send response info with body
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status: resp.StatusCode,
		Body:   string(body),
	})

	var contexts []map[string]any
	textAgg := result.GetCompletionText()

	toolManager := parser.GetToolManager()
	allTools := make([]*ToolExecution, 0)
	for _, tool := range toolManager.GetActiveTools() {
		allTools = append(allTools, tool)
	}
	for _, tool := range toolManager.GetCompletedTools() {
		allTools = append(allTools, tool)
	}

	sawToolUse := len(allTools) > 0

	if textAgg != "" {
		contexts = append(contexts, map[string]any{
			"type": "text",
			"text": textAgg,
		})
	}

	for _, tool := range allTools {
		toolUseBlock := map[string]any{
			"type":  "tool_use",
			"id":    tool.ID,
			"name":  tool.Name,
			"input": tool.Arguments,
		}
		if tool.Arguments == nil {
			toolUseBlock["input"] = map[string]any{}
		}
		contexts = append(contexts, toolUseBlock)
	}

	stopReasonManager := NewStopReasonManager()
	outputTokens := 0
	estimator := NewTokenEstimator()
	for _, contentBlock := range contexts {
		blockType, _ := contentBlock["type"].(string)
		switch blockType {
		case "text":
			if text, ok := contentBlock["text"].(string); ok {
				outputTokens += estimator.EstimateTextTokens(text)
			}
		case "tool_use":
			toolName, _ := contentBlock["name"].(string)
			toolInput, _ := contentBlock["input"].(map[string]any)
			outputTokens += estimator.EstimateToolUseTokens(toolName, toolInput)
		}
	}

	if outputTokens < 1 && len(contexts) > 0 {
		outputTokens = 1
	}

	stopReasonManager.UpdateToolCallStatus(sawToolUse, sawToolUse)
	stopReason := stopReasonManager.DetermineStopReason()

	anthropicResp := map[string]any{
		"content":       contexts,
		"model":         requestModel,
		"role":          "assistant",
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"type":          "message",
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}

	responseBody, err := FastMarshal(anthropicResp)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, false, "failed to encode response")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)

	// Send metrics and response model via EventChannel
	eventChan.SendMetrics(&domain.AdapterMetrics{
		InputTokens:  uint64(inputTokens),
		OutputTokens: uint64(outputTokens),
	})
	eventChan.SendResponseModel(requestModel)

	return nil
}

// collectClaudeSSEToJSON converts Claude SSE events to a single JSON response
func collectClaudeSSEToJSON(sseContent string) ([]byte, error) {
	var messageID, model, stopReason string
	var content []map[string]interface{}
	var inputTokens, outputTokens int

	lines := strings.Split(sseContent, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}

		var event map[string]interface{}
		if err := FastUnmarshal([]byte(data), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)
		switch eventType {
		case "message_start":
			if msg, ok := event["message"].(map[string]interface{}); ok {
				messageID, _ = msg["id"].(string)
				model, _ = msg["model"].(string)
			}

		case "content_block_start":
			if block, ok := event["content_block"].(map[string]interface{}); ok {
				content = append(content, block)
			}

		case "content_block_delta":
			if delta, ok := event["delta"].(map[string]interface{}); ok {
				index := int(event["index"].(float64))
				if index < len(content) {
					deltaType, _ := delta["type"].(string)
					switch deltaType {
					case "text_delta":
						if text, ok := delta["text"].(string); ok {
							if existing, ok := content[index]["text"].(string); ok {
								content[index]["text"] = existing + text
							} else {
								content[index]["text"] = text
							}
						}
					case "input_json_delta":
						if partialJSON, ok := delta["partial_json"].(string); ok {
							if existing, ok := content[index]["input"].(string); ok {
								content[index]["input"] = existing + partialJSON
							} else {
								content[index]["input"] = partialJSON
							}
						}
					}
				}
			}

		case "message_delta":
			if delta, ok := event["delta"].(map[string]interface{}); ok {
				stopReason, _ = delta["stop_reason"].(string)
			}
			if usage, ok := event["usage"].(map[string]interface{}); ok {
				if ot, ok := usage["output_tokens"].(float64); ok {
					outputTokens = int(ot)
				}
			}
		}
	}

	// Parse tool_use input JSON strings
	for i := range content {
		if content[i]["type"] == "tool_use" {
			if inputStr, ok := content[i]["input"].(string); ok {
				var inputObj map[string]interface{}
				if err := FastUnmarshal([]byte(inputStr), &inputObj); err == nil {
					content[i]["input"] = inputObj
				}
			}
		}
	}

	response := map[string]interface{}{
		"id":            messageID,
		"type":          "message",
		"role":          "assistant",
		"content":       content,
		"model":         model,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}

	return FastMarshal(response)
}

// flattenHeaders converts http.Header to map[string]string
func flattenHeaders(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// calculateInputTokens 计算请求的 input token 数量
func calculateInputTokens(requestBody []byte) int {
	var claudeReq converter.ClaudeRequest
	if err := FastUnmarshal(requestBody, &claudeReq); err != nil {
		return 0
	}

	if len(claudeReq.Tools) > 0 {
		filtered := make([]converter.ClaudeTool, 0, len(claudeReq.Tools))
		for _, tool := range claudeReq.Tools {
			if tool.IsWebSearch() {
				continue
			}
			filtered = append(filtered, tool)
		}
		claudeReq.Tools = filtered
	}

	estimator := NewTokenEstimator()
	return estimator.EstimateInputTokens(&claudeReq)
}

// isRetryableStatusCode checks if the status code is retryable
func isRetryableStatusCode(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusRequestTimeout ||
		status >= 500
}

// newKiroHTTPClient creates an HTTP client for Kiro/CodeWhisperer API
// 匹配 kiro2api/utils/client.go:26-52
func newKiroHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			// 连接建立配置 (匹配 kiro2api)
			DialContext: (&net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,

			// TLS配置 (匹配 kiro2api)
			TLSHandshakeTimeout: 15 * time.Second,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				MaxVersion: tls.VersionTLS13,
				CipherSuites: []uint16{
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_AES_128_GCM_SHA256,
				},
			},

			// HTTP配置 (匹配 kiro2api)
			ForceAttemptHTTP2:  false,
			DisableCompression: false,
		},
		// 注意: kiro2api 不设置整体 Timeout
	}
}
