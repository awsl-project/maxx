package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func init() {
	provider.RegisterAdapterFactory("bedrock", NewAdapter)
}

// BedrockAdapter handles communication with AWS Bedrock.
type BedrockAdapter struct {
	provider   *domain.Provider
	httpClient *http.Client
	creds      credentials.StaticCredentialsProvider
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Bedrock == nil {
		return nil, fmt.Errorf("provider %s missing bedrock config", p.Name)
	}
	config := p.Config.Bedrock
	return &BedrockAdapter{
		provider:   p,
		httpClient: newHTTPClient(),
		creds:      credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, ""),
	}, nil
}

func (a *BedrockAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude}
}

func (a *BedrockAdapter) Execute(c *flow.Ctx, provider *domain.Provider) error {
	requestBody := flow.GetRequestBody(c)
	clientWantsStream := flow.GetIsStream(c)
	request := c.Request
	ctx := context.Background()
	if request != nil {
		ctx = request.Context()
	}

	config := provider.Config.Bedrock

	// Resolve region
	region := config.Region
	if region == "" {
		region = DefaultRegion
	}

	// Resolve model prefix: default "us" for cross-region inference
	// Set modelPrefix to "none" in config to disable prefix
	modelPrefix := config.ModelPrefix
	if modelPrefix == "" {
		modelPrefix = DefaultModelPrefix
	} else if modelPrefix == "none" {
		modelPrefix = ""
	}

	// Resolve model ID
	mappedModel := flow.GetMappedModel(c)
	if mappedModel == "" {
		mappedModel = flow.GetRequestModel(c)
	}
	bedrockModelID := resolveModelID(mappedModel, config.ModelMapping, modelPrefix)

	// Sanitize request body for Bedrock
	requestBody = sanitizeRequestBody(requestBody)

	// Build upstream URL
	upstreamURL := buildBedrockURL(region, bedrockModelID, clientWantsStream)

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to create upstream request")
		proxyErr.Scope = domain.ScopeEndpoint
		proxyErr.Reason = domain.CooldownReasonServerError
		return proxyErr
	}

	// Set headers
	upstreamReq.Header.Set("Content-Type", "application/json")
	if clientWantsStream {
		upstreamReq.Header.Set("Accept", "application/vnd.amazon.eventstream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}

	// Sign request with SigV4
	if err := signRequest(ctx, upstreamReq, requestBody, a.creds, region); err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to sign request")
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		return proxyErr
	}

	// Send request info via EventChannel
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  upstreamReq.Method,
			URL:     upstreamURL,
			Headers: sanitizeHeadersForEvent(upstreamReq.Header),
			Body:    string(requestBody),
		})
	}

	// Execute request
	resp, err := a.httpClient.Do(upstreamReq)
	if err != nil {
		proxyErr := domain.NewScopedProxyError(domain.ErrUpstreamError, domain.ScopeProvider, domain.CooldownReasonNetworkError)
		proxyErr.Message = "failed to connect to Bedrock"
		return proxyErr
	}
	defer resp.Body.Close()

	// Handle error responses
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)

		if eventChan := flow.GetEventChan(c); eventChan != nil {
			eventChan.SendResponseInfo(&domain.ResponseInfo{
				Status:  resp.StatusCode,
				Headers: flattenHeaders(resp.Header),
				Body:    string(body),
			})
		}

		return classifyBedrockHTTPError(resp.StatusCode, body, resp.Header, mappedModel)
	}

	// Handle response
	if clientWantsStream {
		return a.handleStreamResponse(c, resp)
	}
	return a.handleNonStreamResponse(c, resp)
}

func (a *BedrockAdapter) handleNonStreamResponse(c *flow.Ctx, resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream response")
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonNetworkError
		return proxyErr
	}

	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    string(body),
		})
		if metrics := usage.ExtractFromResponse(string(body)); metrics != nil {
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:          metrics.InputTokens,
				OutputTokens:         metrics.OutputTokens,
				CacheReadCount:       metrics.CacheReadCount,
				CacheCreationCount:   metrics.CacheCreationCount,
				Cache5mCreationCount: metrics.Cache5mCreationCount,
				Cache1hCreationCount: metrics.Cache1hCreationCount,
			})
		}
		if model := extractModelFromResponse(body); model != "" {
			eventChan.SendResponseModel(model)
		}
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(body)
	return nil
}

func (a *BedrockAdapter) handleStreamResponse(c *flow.Ctx, resp *http.Response) error {
	eventChan := flow.GetEventChan(c)
	if eventChan != nil {
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    "[streaming]",
		})
	}

	// Bedrock streaming returns AWS Event Stream binary framing.
	// We need to parse the event stream and re-emit as SSE.
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

	var collector usage.StreamCollector
	var model string
	firstChunkSent := false
	responseCompleted := false

	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}

	// Parse the AWS Event Stream and extract JSON payloads
	decoder := newEventStreamDecoder(resp.Body)
	for {
		select {
		case <-ctx.Done():
			sendFinalStreamEvents(eventChan, &collector, &model)
			if responseCompleted {
				return nil
			}
			proxyErr := domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
			proxyErr.Scope = domain.ScopeRequest
			return proxyErr
		default:
		}

		payload, err := decoder.Decode()
		if err != nil {
			sendFinalStreamEvents(eventChan, &collector, &model)
			if err == io.EOF || responseCompleted {
				return nil
			}
			if ctx.Err() != nil {
				proxyErr := domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
				proxyErr.Scope = domain.ScopeRequest
				return proxyErr
			}
			proxyErr := domain.NewProxyErrorWithMessage(err, true, "stream read error")
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Reason = domain.CooldownReasonNetworkError
			return proxyErr
		}

		if len(payload) == 0 {
			continue
		}

		// Filter amazon-bedrock-invocationMetrics from message_stop
		payload = filterBedrockMetrics(payload)

		// Format as SSE line (Anthropic API format: "event: " + type from payload)
		eventType := gjson.GetBytes(payload, "type").String()
		if eventType == "" {
			eventType = "message"
		}
		sseLine := "event: " + eventType + "\ndata: " + string(payload) + "\n\n"

		// Process for metrics collection
		collector.ProcessSSELine("data: " + string(payload))
		extractModelFromSSELine("data: "+string(payload), &model)

		if isResponseCompleted(payload) {
			responseCompleted = true
		}

		_, writeErr := c.Writer.Write([]byte(sseLine))
		if writeErr != nil {
			sendFinalStreamEvents(eventChan, &collector, &model)
			if responseCompleted {
				return nil
			}
			proxyErr := domain.NewProxyErrorWithMessage(writeErr, false, "client disconnected")
			proxyErr.Scope = domain.ScopeRequest
			return proxyErr
		}
		flusher.Flush()

		if !firstChunkSent {
			firstChunkSent = true
			if eventChan != nil {
				eventChan.SendFirstToken(time.Now().UnixMilli())
			}
		}
	}
}

// filterBedrockMetrics removes amazon-bedrock-invocationMetrics from message_stop events.
func filterBedrockMetrics(payload []byte) []byte {
	if gjson.GetBytes(payload, "amazon-bedrock-invocationMetrics").Exists() {
		payload, _ = sjson.DeleteBytes(payload, "amazon-bedrock-invocationMetrics")
	}
	return payload
}

func isResponseCompleted(payload []byte) bool {
	return gjson.GetBytes(payload, "type").String() == "message_stop"
}

func extractModelFromSSELine(line string, model *string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	data := strings.TrimPrefix(line, "data: ")
	data = strings.TrimPrefix(data, "data:")
	data = strings.TrimSpace(data)
	if data == "" || data == "[DONE]" {
		return
	}
	if gjson.Valid(data) && gjson.Get(data, "type").String() == "message_start" {
		if m := gjson.Get(data, "message.model").String(); m != "" {
			*model = m
		}
	}
}

func extractModelFromResponse(body []byte) string {
	var resp struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Model != "" {
		return resp.Model
	}
	return ""
}

func sendFinalStreamEvents(eventChan domain.AdapterEventChan, collector *usage.StreamCollector, model *string) {
	if eventChan == nil {
		return
	}
	if collector.Metrics != nil && !collector.Metrics.IsEmpty() {
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:          collector.Metrics.InputTokens,
			OutputTokens:         collector.Metrics.OutputTokens,
			CacheReadCount:       collector.Metrics.CacheReadCount,
			CacheCreationCount:   collector.Metrics.CacheCreationCount,
			Cache5mCreationCount: collector.Metrics.Cache5mCreationCount,
			Cache1hCreationCount: collector.Metrics.Cache1hCreationCount,
		})
	}
	if *model != "" {
		eventChan.SendResponseModel(*model)
	}
}

func classifyBedrockHTTPError(statusCode int, body []byte, headers http.Header, model string) *domain.ProxyError {
	bodyStr := string(body)
	bodyLower := strings.ToLower(bodyStr)

	// Truncate error body to avoid leaking internal AWS details
	errMsg := bodyStr
	if len(errMsg) > 500 {
		errMsg = errMsg[:500] + "..."
	}

	proxyErr := &domain.ProxyError{
		Err:            fmt.Errorf("bedrock error: %s", errMsg),
		Message:        fmt.Sprintf("bedrock returned status %d", statusCode),
		HTTPStatusCode: statusCode,
		Retryable:      isRetryableStatusCode(statusCode),
		ClientType:     string(domain.ClientTypeClaude),
	}

	switch {
	case statusCode == 400:
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Retryable = false
		if strings.Contains(bodyLower, "validationexception") && model != "" &&
			(strings.Contains(bodyLower, "model") || strings.Contains(bodyLower, "inference profile")) {
			proxyErr.Scope = domain.ScopeModel
			proxyErr.Reason = domain.CooldownReasonModelUnavailable
			proxyErr.Model = model
		}

	case statusCode == 403:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		proxyErr.Retryable = false

	case statusCode == 404:
		if model != "" {
			proxyErr.Scope = domain.ScopeModel
			proxyErr.Reason = domain.CooldownReasonModelUnavailable
			proxyErr.Model = model
		} else {
			proxyErr.Scope = domain.ScopeEndpoint
			proxyErr.Reason = domain.CooldownReasonServerError
		}
		proxyErr.Retryable = false

	case statusCode == 429:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonRateLimitExceeded
		proxyErr.Retryable = true
		if retryAfter := headers.Get("Retry-After"); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
				proxyErr.RetryAfter = time.Duration(seconds) * time.Second
				until := time.Now().Add(proxyErr.RetryAfter)
				proxyErr.CooldownUntil = &until
			}
		}
		if proxyErr.CooldownUntil == nil {
			until := time.Now().Add(time.Minute)
			proxyErr.CooldownUntil = &until
		}
		if strings.Contains(bodyLower, "throttlingexception") {
			proxyErr.Reason = domain.CooldownReasonRateLimitExceeded
		}

	case statusCode == 503:
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonServerError
		if strings.Contains(bodyLower, "modelnotreadyexception") {
			proxyErr.Scope = domain.ScopeModel
			proxyErr.Model = model
		}

	case statusCode >= 500:
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonServerError

	default:
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Retryable = false
	}

	return proxyErr
}

func isRetryableStatusCode(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusRequestTimeout,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return status >= 500
	}
}

func flattenHeaders(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

func sanitizeHeadersForEvent(h http.Header) map[string]string {
	result := flattenHeaders(h)
	for _, key := range []string{"Authorization", "X-Amz-Security-Token"} {
		if _, ok := result[key]; ok {
			result[key] = "[REDACTED]"
		}
	}
	return result
}

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   20 * time.Second,
		KeepAlive: 60 * time.Second,
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   600 * time.Second,
	}
}

