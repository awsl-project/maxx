package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

const (
	ClaudeProviderBatchStatusUsable              = "usable"
	ClaudeProviderBatchStatusAuthFailed          = "auth_failed"
	ClaudeProviderBatchStatusModelUnsupported    = "model_unsupported"
	ClaudeProviderBatchStatusTimeout             = "timeout"
	ClaudeProviderBatchStatusUpstream5xx         = "upstream_5xx"
	ClaudeProviderBatchStatusProtocolError       = "maxx_protocol_error"
	ClaudeProviderBatchStatusValidationFailed    = "validation_failed"
	ClaudeProviderBatchStatusDuplicateBlocked    = "duplicate_blocked"
	ClaudeProviderBatchStatusPersistenceFailed   = "persistence_failed"
	ClaudeProviderBatchStatusUnsupportedProvider = "unsupported_provider"

	ClaudeProviderBatchPersistNone        = "none"
	ClaudeProviderBatchPersistPassed      = "passed"
	ClaudeProviderBatchPersistAllDisabled = "all_disabled"
)

// ClaudeProviderBatchRequest drives the Claude routes page bulk provider test/import flow.
// It is intentionally scoped to clientType=claude on the server side so this route-page feature
// cannot accidentally mutate codex/openai/gemini configuration.
type ClaudeProviderBatchRequest struct {
	ExistingProviderIDs []uint64           `json:"existingProviderIDs,omitempty"`
	Candidates          []*domain.Provider `json:"candidates,omitempty"`
	ProjectID           uint64             `json:"projectID"`
	TestModel           string             `json:"testModel"`
	MaxTokens           int                `json:"maxTokens"`
	Concurrency         int                `json:"concurrency"`
	PersistMode         string             `json:"persistMode"`
	CreateRoutes        bool               `json:"createRoutes"`
	OverwriteExisting   bool               `json:"overwriteExisting"`
	RouteWeight         int                `json:"routeWeight"`
}

type ClaudeProviderBatchResponse struct {
	ClientType     domain.ClientType                   `json:"clientType"`
	ProjectID      uint64                              `json:"projectID"`
	TestModel      string                              `json:"testModel"`
	PersistMode    string                              `json:"persistMode"`
	CreateRoutes   bool                                `json:"createRoutes"`
	Concurrency    int                                 `json:"concurrency"`
	Results        []ClaudeProviderBatchProviderResult `json:"results"`
	TestedCount    int                                 `json:"testedCount"`
	UsableCount    int                                 `json:"usableCount"`
	PersistedCount int                                 `json:"persistedCount"`
	RoutesCreated  int                                 `json:"routesCreated"`
	RoutesUpdated  int                                 `json:"routesUpdated"`
	RoutesDisabled int                                 `json:"routesDisabled"`
	RoutesSkipped  int                                 `json:"routesSkipped"`
}

type ClaudeProviderBatchProviderResult struct {
	Index          int               `json:"index"`
	Source         string            `json:"source"`
	ExistingID     uint64            `json:"existingID,omitempty"`
	ProviderID     uint64            `json:"providerID,omitempty"`
	RouteID        uint64            `json:"routeID,omitempty"`
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	BaseURL        string            `json:"baseURL,omitempty"`
	ModelMapping   map[string]string `json:"modelMapping,omitempty"`
	RequestedModel string            `json:"requestedModel"`
	MappedModel    string            `json:"mappedModel"`
	Action         string            `json:"action"`
	Status         string            `json:"status"`
	HTTPStatus     int               `json:"httpStatus,omitempty"`
	OK             bool              `json:"ok"`
	Persisted      bool              `json:"persisted"`
	RouteCreated   bool              `json:"routeCreated"`
	RouteUpdated   bool              `json:"routeUpdated"`
	RouteEnabled   bool              `json:"routeEnabled"`
	Message        string            `json:"message,omitempty"`
	Error          string            `json:"error,omitempty"`
	DurationMS     int64             `json:"durationMs"`
}

type claudeBatchPreparedProvider struct {
	index      int
	source     string
	provider   *domain.Provider
	existingID uint64
	duplicate  *domain.Provider
	result     ClaudeProviderBatchProviderResult
}

func (s *AdminService) ClaudeProviderBatchTest(ctx context.Context, tenantID uint64, req ClaudeProviderBatchRequest) (*ClaudeProviderBatchResponse, error) {
	req = normalizeClaudeProviderBatchRequest(req)
	if len(req.ExistingProviderIDs) == 0 && len(req.Candidates) == 0 {
		return nil, fmt.Errorf("at least one provider is required")
	}

	existingProviders, err := s.providerRepo.List(tenantID)
	if err != nil {
		return nil, err
	}
	prepared := s.prepareClaudeBatchProviders(tenantID, req, existingProviders)

	results := make([]ClaudeProviderBatchProviderResult, len(prepared))
	var wg sync.WaitGroup
	sem := make(chan struct{}, req.Concurrency)
	for i := range prepared {
		item := prepared[i]
		if item.result.Status != "" {
			results[i] = item.result
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				item.result.Status = ClaudeProviderBatchStatusTimeout
				item.result.Error = "request cancelled"
				results[item.index] = item.result
				return
			}
			results[item.index] = testClaudeBatchProvider(ctx, item, req)
		}()
	}
	wg.Wait()

	resp := &ClaudeProviderBatchResponse{
		ClientType:   domain.ClientTypeClaude,
		ProjectID:    req.ProjectID,
		TestModel:    req.TestModel,
		PersistMode:  req.PersistMode,
		CreateRoutes: req.CreateRoutes,
		Concurrency:  req.Concurrency,
		Results:      results,
		TestedCount:  len(results),
	}
	for _, result := range results {
		if result.OK {
			resp.UsableCount++
		}
	}

	if req.PersistMode != ClaudeProviderBatchPersistNone {
		s.persistClaudeBatchResults(tenantID, req, prepared, resp)
	}

	return resp, nil
}

func normalizeClaudeProviderBatchRequest(req ClaudeProviderBatchRequest) ClaudeProviderBatchRequest {
	req.TestModel = strings.TrimSpace(req.TestModel)
	if req.TestModel == "" {
		req.TestModel = "claude-sonnet-4"
	}
	if req.MaxTokens <= 0 || req.MaxTokens > 128 {
		req.MaxTokens = 16
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 4
	}
	if req.Concurrency > 5 {
		req.Concurrency = 5
	}
	if req.PersistMode == "" {
		req.PersistMode = ClaudeProviderBatchPersistPassed
	}
	if req.RouteWeight <= 0 {
		req.RouteWeight = 1
	}
	return req
}

func (s *AdminService) prepareClaudeBatchProviders(tenantID uint64, req ClaudeProviderBatchRequest, existingProviders []*domain.Provider) []*claudeBatchPreparedProvider {
	existingByID := make(map[uint64]*domain.Provider, len(existingProviders))
	existingByName := make(map[string]*domain.Provider, len(existingProviders))
	existingByBase := make(map[string]*domain.Provider, len(existingProviders))
	for _, p := range existingProviders {
		if p == nil {
			continue
		}
		existingByID[p.ID] = p
		existingByName[strings.ToLower(strings.TrimSpace(p.Name))] = p
		if base := normalizedProviderBaseURL(p); base != "" {
			existingByBase[base] = p
		}
	}

	prepared := make([]*claudeBatchPreparedProvider, 0, len(req.ExistingProviderIDs)+len(req.Candidates))
	appendPrepared := func(source string, existingID uint64, provider *domain.Provider, duplicate *domain.Provider, errStatus, errMsg string) {
		idx := len(prepared)
		result := baseClaudeBatchResult(idx, source, existingID, provider, req.TestModel)
		switch {
		case source == "existing":
			result.Action = "test_existing_provider"
		case duplicate != nil && req.OverwriteExisting:
			result.Action = "update_existing_provider"
		case duplicate != nil:
			result.Action = "test_only_duplicate"
		default:
			result.Action = "create_provider"
		}
		if req.CreateRoutes {
			result.Action += "+claude_route"
		}
		if errStatus != "" {
			result.Status = errStatus
			result.Error = errMsg
		}
		prepared = append(prepared, &claudeBatchPreparedProvider{index: idx, source: source, existingID: existingID, provider: provider, duplicate: duplicate, result: result})
	}

	seenExisting := map[uint64]bool{}
	for _, id := range req.ExistingProviderIDs {
		if seenExisting[id] {
			continue
		}
		seenExisting[id] = true
		provider := existingByID[id]
		if provider == nil {
			appendPrepared("existing", id, nil, nil, ClaudeProviderBatchStatusValidationFailed, "provider not found")
			continue
		}
		appendPrepared("existing", id, cloneProviderForBatch(provider), nil, "", "")
	}

	for _, candidate := range req.Candidates {
		candidate = cloneProviderForBatch(candidate)
		if candidate != nil {
			candidate.ID = 0
			candidate.TenantID = tenantID
		}
		var duplicate *domain.Provider
		if candidate != nil {
			if byName := existingByName[strings.ToLower(strings.TrimSpace(candidate.Name))]; byName != nil {
				duplicate = byName
			} else if base := normalizedProviderBaseURL(candidate); base != "" {
				duplicate = existingByBase[base]
			}
		}
		if duplicate != nil && !req.OverwriteExisting {
			appendPrepared("candidate", duplicate.ID, candidate, duplicate, ClaudeProviderBatchStatusDuplicateBlocked, "existing provider matches by name or base URL; enable overwrite to update it")
			continue
		}
		appendPrepared("candidate", 0, candidate, duplicate, "", "")
	}
	return prepared
}

func baseClaudeBatchResult(index int, source string, existingID uint64, provider *domain.Provider, requestedModel string) ClaudeProviderBatchProviderResult {
	result := ClaudeProviderBatchProviderResult{Index: index, Source: source, ExistingID: existingID, RequestedModel: requestedModel}
	if provider != nil {
		result.Name = provider.Name
		result.Type = provider.Type
		result.BaseURL = maskURL(normalizedProviderBaseURL(provider))
		if provider.Config != nil && provider.Config.Custom != nil {
			result.ModelMapping = cloneStringMap(provider.Config.Custom.ModelMapping)
			result.MappedModel = mapClaudeBatchModel(requestedModel, provider.Config.Custom.ModelMapping)
		}
	}
	if result.MappedModel == "" {
		result.MappedModel = requestedModel
	}
	return result
}

func testClaudeBatchProvider(ctx context.Context, item *claudeBatchPreparedProvider, req ClaudeProviderBatchRequest) ClaudeProviderBatchProviderResult {
	result := baseClaudeBatchResult(item.index, item.source, item.existingID, item.provider, req.TestModel)
	if err := validateClaudeBatchProvider(item.provider); err != nil {
		result.Status = ClaudeProviderBatchStatusValidationFailed
		result.Error = err.Error()
		return result
	}

	custom := item.provider.Config.Custom
	mappedModel := mapClaudeBatchModel(req.TestModel, custom.ModelMapping)
	result.MappedModel = mappedModel
	body := map[string]any{
		"model":      mappedModel,
		"max_tokens": req.MaxTokens,
		"messages": []map[string]string{{
			"role":    "user",
			"content": "Reply ok.",
		}},
	}
	payload, _ := json.Marshal(body)
	endpoint := strings.TrimRight(custom.BaseURL, "/") + "/v1/messages"
	if !strings.Contains(endpoint, "?") {
		endpoint += "?beta=true"
	}

	testCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	started := time.Now()
	httpReq, err := http.NewRequestWithContext(testCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		result.Status = ClaudeProviderBatchStatusValidationFailed
		result.Error = "invalid base URL"
		return result
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Anthropic-Version", "2023-06-01")
	httpReq.Header.Set("User-Agent", "maxx-claude-provider-batch-test")
	setClaudeBatchAuthHeader(httpReq, custom.APIKey)

	resp, err := (&http.Client{Timeout: 22 * time.Second}).Do(httpReq)
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") || testCtx.Err() != nil {
			result.Status = ClaudeProviderBatchStatusTimeout
			result.Error = "request timed out"
		} else {
			result.Status = ClaudeProviderBatchStatusProtocolError
			result.Error = sanitizeClaudeBatchError(err.Error())
		}
		return result
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	limitedBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	result.Status, result.OK, result.Message = classifyClaudeBatchResponse(resp.StatusCode, limitedBody)
	if !result.OK {
		result.Error = sanitizeClaudeBatchError(result.Message)
	}
	return result
}

func validateClaudeBatchProvider(provider *domain.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is required")
	}
	if strings.TrimSpace(provider.Name) == "" {
		return fmt.Errorf("provider name is required")
	}
	if provider.Type != "custom" {
		return fmt.Errorf("only custom providers can be tested from Claude routes")
	}
	if provider.Config == nil || provider.Config.Custom == nil {
		return fmt.Errorf("custom provider config is required")
	}
	if strings.TrimSpace(provider.Config.Custom.BaseURL) == "" {
		return fmt.Errorf("base URL is required")
	}
	if strings.TrimSpace(provider.Config.Custom.APIKey) == "" && strings.TrimSpace(provider.Config.Custom.Backend) != "ollama" {
		return fmt.Errorf("API key is required")
	}
	if !providerSupportsClient(provider, domain.ClientTypeClaude) {
		return fmt.Errorf("provider does not support claude")
	}
	return nil
}

func classifyClaudeBatchResponse(status int, body []byte) (string, bool, string) {
	if status >= 200 && status < 300 {
		if claudeBatchBodyHasContent(body) {
			return ClaudeProviderBatchStatusUsable, true, "provider returned content"
		}
		return ClaudeProviderBatchStatusProtocolError, false, "HTTP 2xx response did not contain Claude content"
	}
	message := extractClaudeBatchErrorMessage(body)
	lower := strings.ToLower(message)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ClaudeProviderBatchStatusAuthFailed, false, message
	case status == http.StatusBadRequest || status == http.StatusNotFound:
		if strings.Contains(lower, "model") || strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist") {
			return ClaudeProviderBatchStatusModelUnsupported, false, message
		}
		return ClaudeProviderBatchStatusProtocolError, false, message
	case status >= 500:
		return ClaudeProviderBatchStatusUpstream5xx, false, message
	default:
		return ClaudeProviderBatchStatusProtocolError, false, message
	}
}

func claudeBatchBodyHasContent(body []byte) bool {
	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		ID    string `json:"id"`
		Error any    `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	if payload.Error != nil {
		return false
	}
	if len(payload.Content) == 0 {
		return payload.ID != ""
	}
	for _, part := range payload.Content {
		if strings.TrimSpace(part.Text) != "" || strings.TrimSpace(part.Type) != "" {
			return true
		}
	}
	return false
}

func extractClaudeBatchErrorMessage(body []byte) string {
	if len(body) == 0 {
		return "empty error response"
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if msg := extractStringFromClaudeBatchPayload(payload, "message"); msg != "" {
			return msg
		}
		if errObj, ok := payload["error"].(map[string]any); ok {
			if msg := extractStringFromClaudeBatchPayload(errObj, "message"); msg != "" {
				return msg
			}
			if typ := extractStringFromClaudeBatchPayload(errObj, "type"); typ != "" {
				return typ
			}
		}
	}
	return string(bytes.TrimSpace(body))
}

func extractStringFromClaudeBatchPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func (s *AdminService) persistClaudeBatchResults(tenantID uint64, req ClaudeProviderBatchRequest, prepared []*claudeBatchPreparedProvider, resp *ClaudeProviderBatchResponse) {
	routes, _ := s.routeRepo.List(tenantID)
	nextPosition := nextClaudeBatchRoutePosition(routes, req.ProjectID)
	for i := range resp.Results {
		result := &resp.Results[i]
		item := prepared[i]
		shouldPersistProvider := result.OK || req.PersistMode == ClaudeProviderBatchPersistAllDisabled
		if !shouldPersistProvider || result.Status == ClaudeProviderBatchStatusDuplicateBlocked || result.Status == ClaudeProviderBatchStatusValidationFailed {
			continue
		}
		provider := cloneProviderForBatch(item.provider)
		if provider == nil {
			continue
		}
		var providerID uint64
		if item.source == "existing" {
			providerID = item.existingID
		} else if item.duplicate != nil && req.OverwriteExisting {
			provider.ID = item.duplicate.ID
			provider.TenantID = tenantID
			provider.CreatedAt = item.duplicate.CreatedAt
			if err := s.UpdateProvider(tenantID, provider); err != nil {
				result.Status = ClaudeProviderBatchStatusPersistenceFailed
				result.OK = false
				result.Error = sanitizeClaudeBatchError(err.Error())
				continue
			}
			providerID = provider.ID
			result.Persisted = true
			resp.PersistedCount++
		} else {
			provider.ID = 0
			provider.TenantID = tenantID
			if err := s.CreateProvider(tenantID, provider); err != nil {
				result.Status = ClaudeProviderBatchStatusPersistenceFailed
				result.OK = false
				result.Error = sanitizeClaudeBatchError(err.Error())
				continue
			}
			providerID = provider.ID
			result.Persisted = true
			resp.PersistedCount++
		}
		result.ProviderID = providerID

		if !req.CreateRoutes || providerID == 0 {
			continue
		}
		enabled := result.OK
		if req.PersistMode == ClaudeProviderBatchPersistAllDisabled && !result.OK {
			enabled = false
		}
		route, err := s.routeRepo.FindByKey(tenantID, req.ProjectID, providerID, domain.ClientTypeClaude)
		if err == nil && route != nil {
			route.IsEnabled = enabled
			route.Weight = req.RouteWeight
			if route.Position <= 0 {
				route.Position = nextPosition
				nextPosition++
			}
			if err := s.UpdateRoute(tenantID, route); err != nil {
				result.Error = sanitizeClaudeBatchError(err.Error())
				continue
			}
			result.RouteID = route.ID
			result.RouteUpdated = true
			result.RouteEnabled = route.IsEnabled
			resp.RoutesUpdated++
			if !route.IsEnabled {
				resp.RoutesDisabled++
			}
			continue
		}

		route = &domain.Route{
			TenantID:   tenantID,
			IsEnabled:  enabled,
			IsNative:   false,
			ProjectID:  req.ProjectID,
			ClientType: domain.ClientTypeClaude,
			ProviderID: providerID,
			Position:   nextPosition,
			Weight:     req.RouteWeight,
		}
		nextPosition++
		if err := s.CreateRoute(tenantID, route); err != nil {
			result.Error = sanitizeClaudeBatchError(err.Error())
			continue
		}
		result.RouteID = route.ID
		result.RouteCreated = true
		result.RouteEnabled = route.IsEnabled
		resp.RoutesCreated++
		if !route.IsEnabled {
			resp.RoutesDisabled++
		}
	}
	for _, result := range resp.Results {
		if req.CreateRoutes && result.ProviderID != 0 && !result.RouteCreated && !result.RouteUpdated {
			resp.RoutesSkipped++
		}
	}
}

func nextClaudeBatchRoutePosition(routes []*domain.Route, projectID uint64) int {
	positions := make([]int, 0, len(routes))
	for _, route := range routes {
		if route != nil && route.ClientType == domain.ClientTypeClaude && route.ProjectID == projectID {
			positions = append(positions, route.Position)
		}
	}
	if len(positions) == 0 {
		return 1
	}
	sort.Ints(positions)
	return positions[len(positions)-1] + 1
}

func providerSupportsClient(provider *domain.Provider, clientType domain.ClientType) bool {
	if provider == nil {
		return false
	}
	if len(provider.SupportedClientTypes) == 0 {
		return provider.Type == "custom"
	}
	for _, supported := range provider.SupportedClientTypes {
		if supported == clientType {
			return true
		}
	}
	return false
}

func normalizedProviderBaseURL(provider *domain.Provider) string {
	if provider == nil || provider.Config == nil || provider.Config.Custom == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(provider.Config.Custom.BaseURL), "/")
}

func mapClaudeBatchModel(requested string, mapping map[string]string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "claude-sonnet-4"
	}
	if mapped := strings.TrimSpace(mapping[requested]); mapped != "" {
		return mapped
	}
	if mapped := strings.TrimSpace(mapping["*"]); mapped != "" {
		return mapped
	}
	return requested
}

func setClaudeBatchAuthHeader(req *http.Request, apiKey string) {
	if strings.TrimSpace(apiKey) == "" {
		return
	}
	if req.URL != nil && strings.EqualFold(req.URL.Scheme, "https") && strings.EqualFold(req.URL.Host, "api.anthropic.com") {
		req.Header.Set("x-api-key", apiKey)
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

func sanitizeClaudeBatchError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	redactors := []string{"api_key", "apiKey", "token", "secret", "password", "authorization", "x-api-key"}
	for _, marker := range redactors {
		message = redactMarkerValue(message, marker)
	}
	if len(message) > 500 {
		message = message[:500] + "…"
	}
	return message
}

func redactMarkerValue(input, marker string) string {
	lower := strings.ToLower(input)
	needle := strings.ToLower(marker)
	idx := strings.Index(lower, needle)
	if idx < 0 {
		return input
	}
	end := idx + len(marker)
	for end < len(input) && strings.ContainsRune(" \t:=\"'", rune(input[end])) {
		end++
	}
	valueEnd := end
	for valueEnd < len(input) && !strings.ContainsRune(" \t\r\n,;}\"'", rune(input[valueEnd])) {
		valueEnd++
	}
	if valueEnd <= end {
		return input
	}
	return input[:end] + "[REDACTED]" + input[valueEnd:]
}

func maskURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}

func cloneProviderForBatch(provider *domain.Provider) *domain.Provider {
	if provider == nil {
		return nil
	}
	clone := *provider
	clone.SupportedClientTypes = append([]domain.ClientType(nil), provider.SupportedClientTypes...)
	clone.SupportModels = append([]string(nil), provider.SupportModels...)
	if provider.Config != nil {
		cfg := *provider.Config
		if provider.Config.Custom != nil {
			custom := *provider.Config.Custom
			custom.ClientBaseURL = cloneClientBaseURLMap(provider.Config.Custom.ClientBaseURL)
			custom.ModelMapping = cloneStringMap(provider.Config.Custom.ModelMapping)
			custom.ResponseModelMapping = cloneStringMap(provider.Config.Custom.ResponseModelMapping)
			cfg.Custom = &custom
		}
		clone.Config = &cfg
	}
	return &clone
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneClientBaseURLMap(source map[domain.ClientType]string) map[domain.ClientType]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[domain.ClientType]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
