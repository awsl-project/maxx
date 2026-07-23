package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

const defaultRouteTTFTProbeTimeout = 30 * time.Second

type RouteTTFTProbeRequest struct {
	RouteIDs    []uint64          `json:"routeIDs"`
	ClientType  domain.ClientType `json:"clientType"`
	ProjectID   uint64            `json:"projectID"`
	TestModel   string            `json:"testModel,omitempty"`
	Concurrency int               `json:"concurrency,omitempty"`
	TimeoutMS   int               `json:"timeoutMs,omitempty"`
}

type RouteTTFTProbeResponse struct {
	ClientType  domain.ClientType      `json:"clientType"`
	ProjectID   uint64                 `json:"projectID"`
	TestModel   string                 `json:"testModel"`
	Concurrency int                    `json:"concurrency"`
	Results     []RouteTTFTProbeResult `json:"results"`
}

type RouteTTFTProbeResult struct {
	RouteID      uint64 `json:"routeID"`
	ProviderID   uint64 `json:"providerID"`
	ProviderName string `json:"providerName"`
	OK           bool   `json:"ok"`
	Status       string `json:"status"`
	Metric       string `json:"metric"`
	TTFTMS       int64  `json:"ttftMs,omitempty"`
	DurationMS   int64  `json:"durationMs"`
	HTTPStatus   int    `json:"httpStatus,omitempty"`
	Error        string `json:"error,omitempty"`
}

type routeTTFTProbeWriter struct {
	header     http.Header
	statusCode int
	firstWrite time.Time
	bytes      int
	mu         sync.Mutex
}

func newRouteTTFTProbeWriter() *routeTTFTProbeWriter {
	return &routeTTFTProbeWriter{header: make(http.Header), statusCode: http.StatusOK}
}

func (w *routeTTFTProbeWriter) Header() http.Header { return w.header }

func (w *routeTTFTProbeWriter) WriteHeader(statusCode int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.statusCode == 0 || w.statusCode == http.StatusOK {
		w.statusCode = statusCode
	}
}

func (w *routeTTFTProbeWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.firstWrite.IsZero() {
		w.firstWrite = time.Now()
	}
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	w.bytes += len(b)
	return len(b), nil
}

func (w *routeTTFTProbeWriter) Flush() {}

func (h *AdminHandler) handleRouteTTFTProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.providerProxyHandler == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "provider proxy handler not configured"})
		return
	}

	var req RouteTTFTProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	resp, err := h.providerProxyHandler.ProbeRoutesTTFT(r.Context(), maxxctx.GetTenantID(r.Context()), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *ProviderProxyHandler) ProbeRoutesTTFT(ctx context.Context, tenantID uint64, req RouteTTFTProbeRequest) (*RouteTTFTProbeResponse, error) {
	if h == nil || h.proxyHandler == nil || h.proxyHandler.executor == nil {
		return nil, fmt.Errorf("provider proxy executor not configured")
	}
	if req.ClientType == "" {
		return nil, fmt.Errorf("clientType is required")
	}
	if len(req.RouteIDs) == 0 {
		return nil, fmt.Errorf("routeIDs are required")
	}
	model := probeDefaultModel(req.ClientType, req.TestModel)
	if model == "" {
		return nil, fmt.Errorf("unsupported clientType: %s", req.ClientType)
	}
	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > 4 {
		concurrency = 4
	}
	timeout := defaultRouteTTFTProbeTimeout
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}
	if timeout > defaultRouteTTFTProbeTimeout {
		timeout = defaultRouteTTFTProbeTimeout
	}

	items := make([]struct {
		index    int
		route    *domain.Route
		provider *domain.Provider
	}, 0, len(req.RouteIDs))
	for index, routeID := range req.RouteIDs {
		route, err := h.routeRepo.GetByID(tenantID, routeID)
		if err != nil {
			return nil, err
		}
		if route == nil {
			return nil, fmt.Errorf("route %d not found", routeID)
		}
		if route.ClientType != req.ClientType || route.ProjectID != req.ProjectID {
			return nil, fmt.Errorf("route %d does not belong to requested scope", routeID)
		}
		provider, err := h.providerRepo.GetByID(tenantID, route.ProviderID)
		if err != nil {
			return nil, err
		}
		if provider == nil {
			return nil, fmt.Errorf("provider %d not found", route.ProviderID)
		}
		items = append(items, struct {
			index    int
			route    *domain.Route
			provider *domain.Provider
		}{index: index, route: route, provider: provider})
	}

	results := make([]RouteTTFTProbeResult, len(items))
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for _, item := range items {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[item.index] = routeTTFTProbeError(item.route, item.provider, "cancelled", "request cancelled", 0, 0)
				return
			}
			probeCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			results[item.index] = h.probeRouteTTFT(probeCtx, tenantID, req.ProjectID, req.ClientType, model, item.route, item.provider)
		}()
	}
	wg.Wait()

	sort.SliceStable(results, func(i, j int) bool { return results[i].RouteID < results[j].RouteID })
	return &RouteTTFTProbeResponse{ClientType: req.ClientType, ProjectID: req.ProjectID, TestModel: model, Concurrency: concurrency, Results: results}, nil
}

func (h *ProviderProxyHandler) probeRouteTTFT(ctx context.Context, tenantID uint64, projectID uint64, clientType domain.ClientType, model string, route *domain.Route, provider *domain.Provider) RouteTTFTProbeResult {
	body, apiPath, err := buildRouteTTFTProbeBody(clientType, model)
	if err != nil {
		return routeTTFTProbeError(route, provider, "unsupported", err.Error(), 0, 0)
	}
	started := time.Now()
	req, err := http.NewRequestWithContext(maxxctx.WithTenantID(ctx, tenantID), http.MethodPost, apiPath, bytes.NewReader(body))
	if err != nil {
		return routeTTFTProbeError(route, provider, "validation_failed", err.Error(), 0, 0)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	req.Header.Set("X-Maxx-TTFT-Probe", "true")
	writer := newRouteTTFTProbeWriter()
	c := flow.NewCtx(writer, req)
	c.Set(flow.KeyProxyContext, req.Context())
	c.Set(flow.KeyClientType, clientType)
	c.Set(flow.KeySessionID, fmt.Sprintf("ttft-probe-%d-%d", route.ID, time.Now().UnixNano()))
	c.Set(flow.KeyRequestModel, model)
	c.Set(flow.KeyRequestBody, body)
	c.Set(flow.KeyOriginalRequestBody, body)
	c.Set(flow.KeyRequestHeaders, req.Header)
	c.Set(flow.KeyRequestURI, apiPath)
	c.Set(flow.KeyIsStream, true)
	c.Set(flow.KeyProjectID, projectID)
	c.Set(flow.KeyAPITokenID, uint64(0))

	matchedRoute, err := h.proxyHandler.executor.MatchProviderProxyRoute(req.Context(), tenantID, provider.ID, clientType, projectID, model, 0, flow.GetSessionID(c))
	if err != nil || matchedRoute == nil || matchedRoute.Route == nil || matchedRoute.Provider == nil || matchedRoute.ProviderAdapter == nil {
		return routeTTFTProbeError(route, provider, "route_unavailable", fmt.Sprintf("provider route not found: %v", err), 0, time.Since(started).Milliseconds())
	}
	mappedModel := h.proxyHandler.executor.MapModelForProviderProxy(tenantID, model, matchedRoute.Route, matchedRoute.Provider, clientType, projectID, 0)
	c.Set(flow.KeyMappedModel, mappedModel)
	proxyReq := h.newProxyRequest(c, matchedRoute.Route, matchedRoute.Provider, model, mappedModel, true, true)
	if err := h.proxyHandler.executor.ExecuteProviderProxyMatched(c, proxyReq, matchedRoute); err != nil {
		return routeTTFTProbeError(route, provider, "network_error", err.Error(), writer.statusCode, time.Since(started).Milliseconds())
	}
	if writer.statusCode >= 400 {
		return routeTTFTProbeError(route, provider, "http_error", http.StatusText(writer.statusCode), writer.statusCode, time.Since(started).Milliseconds())
	}
	if writer.firstWrite.IsZero() {
		return routeTTFTProbeError(route, provider, "protocol_error", "upstream returned no response body", writer.statusCode, time.Since(started).Milliseconds())
	}
	duration := time.Since(started).Milliseconds()
	return RouteTTFTProbeResult{RouteID: route.ID, ProviderID: provider.ID, ProviderName: provider.Name, OK: true, Status: "success", Metric: "ttft", TTFTMS: writer.firstWrite.Sub(started).Milliseconds(), DurationMS: duration, HTTPStatus: writer.statusCode}
}

func routeTTFTProbeError(route *domain.Route, provider *domain.Provider, status, message string, httpStatus int, durationMS int64) RouteTTFTProbeResult {
	providerID := uint64(0)
	providerName := ""
	if provider != nil {
		providerID = provider.ID
		providerName = provider.Name
	}
	return RouteTTFTProbeResult{RouteID: route.ID, ProviderID: providerID, ProviderName: providerName, OK: false, Status: status, Metric: "none", DurationMS: durationMS, HTTPStatus: httpStatus, Error: message}
}

func probeDefaultModel(clientType domain.ClientType, override string) string {
	if override != "" {
		return override
	}
	switch clientType {
	case domain.ClientTypeClaude:
		return "claude-sonnet-4"
	case domain.ClientTypeOpenAI:
		return "gpt-4o-mini"
	case domain.ClientTypeCodex:
		return "gpt-5"
	case domain.ClientTypeGemini:
		return "gemini-2.5-flash"
	default:
		return ""
	}
}

func buildRouteTTFTProbeBody(clientType domain.ClientType, model string) ([]byte, string, error) {
	var payload any
	switch clientType {
	case domain.ClientTypeClaude:
		payload = map[string]any{"model": model, "max_tokens": 1, "stream": true, "messages": []map[string]string{{"role": "user", "content": "Reply ok."}}}
		body, err := json.Marshal(payload)
		return body, "/v1/messages", err
	case domain.ClientTypeOpenAI:
		payload = map[string]any{"model": model, "max_tokens": 1, "stream": true, "messages": []map[string]string{{"role": "user", "content": "Reply ok."}}}
		body, err := json.Marshal(payload)
		return body, "/v1/chat/completions", err
	case domain.ClientTypeCodex:
		payload = map[string]any{"model": model, "stream": true, "max_output_tokens": 1, "input": "Reply ok."}
		body, err := json.Marshal(payload)
		return body, "/v1/responses", err
	case domain.ClientTypeGemini:
		payload = map[string]any{"contents": []map[string]any{{"role": "user", "parts": []map[string]string{{"text": "Reply ok."}}}}, "generationConfig": map[string]int{"maxOutputTokens": 1}}
		body, err := json.Marshal(payload)
		return body, fmt.Sprintf("/v1beta/models/%s:generateContent", model), err
	default:
		return nil, "", fmt.Errorf("unsupported clientType: %s", clientType)
	}
}

var _ http.Flusher = (*routeTTFTProbeWriter)(nil)
