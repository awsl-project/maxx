package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func init() {
	provider.RegisterAdapterFactory("bedrock", NewAdapter)
}

// discoveryRepo is the persistence backend for the profileDiscoverer,
// shared by every BedrockAdapter instance. Wired at process startup by
// the binary that owns the database; stays nil in unit tests, in which
// case discovery falls back to in-memory-only behaviour. Set once at
// boot — reassigning under concurrent NewAdapter calls is undefined.
var discoveryRepo repository.BedrockDiscoveryRepository

// SetDiscoveryRepository wires the per-process persistence backend for
// Bedrock discovery. Called once from main after the sqlite layer is
// available. Passing nil disables persistence.
func SetDiscoveryRepository(r repository.BedrockDiscoveryRepository) {
	discoveryRepo = r
}

// BedrockAdapter handles communication with AWS Bedrock.
type BedrockAdapter struct {
	provider   *domain.Provider
	httpClient *http.Client
	creds      credentials.StaticCredentialsProvider
	discoverer *profileDiscoverer
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Bedrock == nil {
		return nil, fmt.Errorf("provider %s missing bedrock config", p.Name)
	}
	config := p.Config.Bedrock
	creds := credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, "")
	region := config.Region
	if region == "" {
		region = DefaultRegion
	}
	httpClient := newHTTPClient()
	discoverer := newProfileDiscoverer(httpClient, creds, region)
	if discoveryRepo != nil && p.ID > 0 {
		discoverer.repo = discoveryRepo
		discoverer.providerID = p.ID
		// Access-key ID (not the secret) identifies the IAM principal;
		// storing it alongside region lets loadFromStore reject rows
		// from a previous config when the operator rotates credentials
		// or retargets the provider at another account/region.
		discoverer.accessKeyID = config.AccessKeyID
		// Warm the in-memory cache from the persisted catalog so the
		// first request doesn't pay ~1-5s of AWS round-trip latency.
		// If the persisted FetchedAt is older than the TTL, Lookup
		// will refresh on demand — loadFromStore only primes, it does
		// not extend the TTL.
		discoverer.loadFromStore()
	}
	return &BedrockAdapter{
		provider:   p,
		httpClient: httpClient,
		creds:      creds,
		discoverer: discoverer,
	}, nil
}

func (a *BedrockAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude}
}

// DiscoveredModel describes one entry in the discovery catalog: the
// client-facing short name, the invoke-ready Bedrock ID, and which
// upstream catalog it came from. Source is either "inference-profile"
// (cross-region, ID carries a region prefix) or "foundation-model"
// (bare anthropic.X, single-region).
type DiscoveredModel struct {
	ShortName string `json:"shortName"`
	BedrockID string `json:"bedrockId"`
	Source    string `json:"source"`
}

// DiscoveredModelsResult is the payload returned by the admin endpoint.
// Available distinguishes "discovery completed, the listed models are
// what this provider can currently invoke" from "discovery never
// succeeded — usually missing bedrock:ListInferenceProfiles IAM
// permission"; Region echoes back where the lookup ran so the UI can
// show it alongside the list.
type DiscoveredModelsResult struct {
	Available bool              `json:"available"`
	Region    string            `json:"region"`
	Models    []DiscoveredModel `json:"models"`
}

// RefreshDiscoveredModels forces a fresh ListInferenceProfiles +
// ListFoundationModels round-trip, bypassing the normal TTL and the
// Invalidate() rate-limit. Used by the admin UI's "refresh" button —
// the operator's intent is explicit, so the rate-limit that protects
// against error-path stampedes is not needed here. Returns the refreshed
// catalog and any fetch error (the catalog still reflects whatever was
// in memory when the fetch failed).
func (a *BedrockAdapter) RefreshDiscoveredModels(ctx context.Context) (DiscoveredModelsResult, error) {
	var refreshErr error
	if a.discoverer != nil {
		lookupCtx, cancel := context.WithTimeout(ctx, discoveryLookupTimeout)
		defer cancel()
		refreshErr = a.discoverer.ForceRefresh(lookupCtx)
	}
	// Refresh path already forced a fetch; pass false so DiscoveredModels
	// doesn't do a redundant second Lookup.
	return a.DiscoveredModels(ctx, false), refreshErr
}

// DiscoveredModels returns the current catalog. When allowRefresh is
// true, a TTL-expiry Lookup is triggered so the caller gets a fresh
// list if the cache is stale; when false, the in-memory snapshot is
// returned as-is. Admin GETs pass true (they're fine paying an
// occasional AWS round-trip); non-admin GETs from the self-service
// surface pass false, so an anonymous poller cannot wait out the TTL
// to force a ListInferenceProfiles call. Uses an isolated background
// context on the refresh path so admin UI polling can't poison the
// shared cache if the client disconnects.
func (a *BedrockAdapter) DiscoveredModels(ctx context.Context, allowRefresh bool) DiscoveredModelsResult {
	region := DefaultRegion
	if a.provider != nil && a.provider.Config != nil && a.provider.Config.Bedrock != nil && a.provider.Config.Bedrock.Region != "" {
		region = a.provider.Config.Bedrock.Region
	}
	result := DiscoveredModelsResult{Region: region}
	if a.discoverer == nil {
		return result
	}

	if allowRefresh {
		lookupCtx, cancel := context.WithTimeout(ctx, discoveryLookupTimeout)
		defer cancel()
		// Force a refresh if the cache is stale. Lookup's argument is a
		// miss-on-purpose key — we only want the side effect.
		_, _ = a.discoverer.Lookup(lookupCtx, "__admin_refresh__")
	}

	entries := a.discoverer.Entries()
	result.Available = a.discoverer.Available()
	result.Models = make([]DiscoveredModel, 0, len(entries))
	for _, e := range entries {
		// Source is carried from the AWS catalog that produced the
		// entry, not inferred from the ID shape — profile-shaped IDs
		// can appear unprefixed from ListFoundationModels too.
		result.Models = append(result.Models, DiscoveredModel{
			ShortName: e.ShortName,
			BedrockID: e.BedrockID,
			Source:    e.Source,
		})
	}
	sort.Slice(result.Models, func(i, j int) bool {
		return result.Models[i].ShortName < result.Models[j].ShortName
	})
	return result
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
	lookup := func(name string) (string, bool) {
		if a.discoverer == nil {
			return "", false
		}
		// Decouple discovery from the request context: a client disconnect
		// must not cancel an in-flight ListInferenceProfiles call and poison
		// the shared cache. Give discovery its own bounded timeout.
		discoveryCtx, cancel := context.WithTimeout(context.Background(), discoveryLookupTimeout)
		defer cancel()
		return a.discoverer.Lookup(discoveryCtx, name)
	}
	bedrockModelID, ok := resolveModelID(mappedModel, config.ModelMapping, modelPrefix, lookup)
	if !ok {
		return a.unresolvableModelError(mappedModel, region)
	}

	// Sanitize request body for Bedrock
	requestBody = sanitizeRequestBody(requestBody)

	// Model-specific thinking config: Opus 4.7 rejects the classic
	// thinking.type="enabled" shape and requires adaptive. Earlier
	// sanitize steps run without knowing the target model; this step
	// rewrites the thinking block (and sets output_config.effort) based
	// on the resolved Bedrock ID's short name, so classic-shape clients
	// (e.g. Claude Code CLI) can still hit adaptive-only models.
	if short, _, ok := extractNameAndDate(bedrockModelID); ok {
		requestBody = adaptThinkingForModel(requestBody, short)
	}

	// Build upstream URL
	upstreamURL := buildBedrockURL(region, bedrockModelID, clientWantsStream)

	// Up to two attempts: a Bedrock 400 that rejects a thinking-block
	// envelope (signature on `thinking`, opaque `data` on
	// `redacted_thinking`) is recoverable by stripping those blocks
	// and replaying once. Cross-deployment replays produced by
	// clients that captured a transcript against Anthropic and now
	// hit Bedrock are the common cause; retry preserves the rest of
	// the conversation rather than failing the whole request.
	//
	// Streaming requests are covered by the same path: Bedrock
	// validates the request body before opening the response stream,
	// so envelope rejections come back as a non-streaming HTTP 400
	// even when the client asked for a stream — they hit this branch,
	// not handleStreamResponse.
	var resp *http.Response
	thinkingRetried := false
	for {
		var attemptErr error
		resp, attemptErr = a.sendBedrockRequest(ctx, c, upstreamURL, requestBody, region, clientWantsStream)
		if attemptErr != nil {
			return attemptErr
		}

		if resp.StatusCode < 400 {
			break
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if !thinkingRetried && resp.StatusCode == 400 && IsThinkingBlockEnvelopeError(body) {
			stripped := StripThinkingBlocks(requestBody)
			if !bytes.Equal(stripped, requestBody) {
				// Note: the swallowed 400 is intentionally not emitted
				// via SendResponseInfo. The executor's attempt record
				// holds a single ResponseInfo slot, so a successful
				// retry would overwrite the 400 anyway and the persisted
				// trace would just look like a one-shot success. Surfacing
				// the recovered-from error properly needs a multi-attempt
				// schema, which is out of scope for this change.
				requestBody = stripped
				thinkingRetried = true
				continue
			}
		}

		if eventChan := flow.GetEventChan(c); eventChan != nil {
			eventChan.SendResponseInfo(&domain.ResponseInfo{
				Status:  resp.StatusCode,
				Headers: flattenHeaders(resp.Header),
				Body:    string(body),
			})
		}

		proxyErr := classifyBedrockHTTPError(resp.StatusCode, body, resp.Header, mappedModel)
		// When the upstream rejects our model ID, AWS may have rotated legacy
		// profiles. Invalidate the discovery cache so the *next* request
		// reloads from ListInferenceProfiles and picks up the new mapping.
		if a.discoverer != nil && proxyErr != nil && proxyErr.Reason == domain.CooldownReasonModelUnavailable {
			a.discoverer.Invalidate()
		}
		return proxyErr
	}
	defer resp.Body.Close()

	// Handle response
	if clientWantsStream {
		return a.handleStreamResponse(c, resp)
	}
	return a.handleNonStreamResponse(c, resp)
}

// sendBedrockRequest builds, signs, emits the request-info event for,
// and sends one upstream HTTP request. Factored out of Execute so the
// thinking-block retry path can replay with a fresh request without
// duplicating the SigV4/event plumbing. Caller owns the returned
// response body.
func (a *BedrockAdapter) sendBedrockRequest(ctx context.Context, c *flow.Ctx, upstreamURL string, requestBody []byte, region string, clientWantsStream bool) (*http.Response, error) {
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to create upstream request")
		proxyErr.Scope = domain.ScopeEndpoint
		proxyErr.Reason = domain.CooldownReasonServerError
		return nil, proxyErr
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	if clientWantsStream {
		upstreamReq.Header.Set("Accept", "application/vnd.amazon.eventstream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}

	if err := signRequest(ctx, upstreamReq, requestBody, a.creds, region); err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to sign request")
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		return nil, proxyErr
	}

	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  upstreamReq.Method,
			URL:     upstreamURL,
			Headers: sanitizeHeadersForEvent(upstreamReq.Header),
			Body:    string(requestBody),
		})
	}

	resp, err := a.httpClient.Do(upstreamReq)
	if err != nil {
		proxyErr := domain.NewScopedProxyError(domain.ErrUpstreamError, domain.ScopeProvider, domain.CooldownReasonNetworkError)
		proxyErr.Message = "failed to connect to Bedrock"
		return nil, proxyErr
	}
	return resp, nil
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

// unresolvableModelError builds a ScopeModel error when the request model
// cannot be resolved to a Bedrock ID. The message differs depending on
// whether discovery is usable, so the operator knows whether to grant IAM
// permission or just add a modelMapping entry.
func (a *BedrockAdapter) unresolvableModelError(model, region string) *domain.ProxyError {
	var msg string
	if a.discoverer != nil && a.discoverer.Available() {
		names := a.discoverer.Names()
		sort.Strings(names)
		msg = fmt.Sprintf("model %q is not available on Bedrock in region %s; available: [%s]",
			model, region, strings.Join(names, ", "))
	} else {
		msg = fmt.Sprintf("cannot resolve model %q: Bedrock discovery unavailable "+
			"(grant bedrock:ListInferenceProfiles or configure bedrock.modelMapping)", model)
	}
	proxyErr := domain.NewProxyErrorWithMessage(errors.New(msg), false, msg)
	proxyErr.Scope = domain.ScopeModel
	proxyErr.Reason = domain.CooldownReasonModelUnavailable
	proxyErr.Model = model
	proxyErr.ClientType = string(domain.ClientTypeClaude)
	proxyErr.HTTPStatusCode = http.StatusBadRequest
	return proxyErr
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
		// Only upgrade to ScopeModel for genuine model-availability errors.
		// Use specific Bedrock error patterns to avoid false positives from
		// field validation errors that happen to mention "model".
		if model != "" && isBedrockModelUnavailable(bodyStr) {
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

// isBedrockModelUnavailable checks whether a Bedrock 400 error body indicates
// the model itself is unavailable (as opposed to a request validation error
// that happens to mention the word "model"). This prevents false-positive
// ScopeModel classification that would wrongly freeze the provider.
func isBedrockModelUnavailable(body string) bool {
	bodyLower := strings.ToLower(body)
	// Bedrock model-access errors use specific exception types and phrases.
	modelPatterns := []string{
		"could not resolve the foundation model",
		"is not authorized to perform: bedrock",
		"you don't have access to the model",
		"access denied for model",
		"model identifier is invalid",
		"inference profile",
	}
	for _, p := range modelPatterns {
		if strings.Contains(bodyLower, p) {
			return true
		}
	}
	return false
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

