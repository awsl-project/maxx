package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

func (e *Executor) ProbeModelHealth(ctx context.Context, tenantID uint64, apiTokenID uint64, target domain.ModelHealthCheckTarget) domain.ModelHealthProbeResult {
	started := time.Now()
	if e == nil || e.router == nil {
		return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusError, Error: "executor unavailable"}
	}
	if target.Model == "" || target.RouteID == 0 || target.ProviderID == 0 {
		return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusError, Error: "invalid health target"}
	}
	result, err := e.router.Match(&router.MatchContext{
		Ctx:          ctx,
		TenantID:     tenantID,
		ClientType:   domain.ClientTypeOpenAI,
		RequestModel: target.Model,
		ModelCandidates: func(route *domain.Route, provider *domain.Provider, clientType domain.ClientType, requestModel string) []string {
			return e.mapModelCandidates(tenantID, requestModel, route, provider, clientType, 0, apiTokenID)
		},
		APITokenID:          apiTokenID,
		StrictSupportModels: true,
	})
	if err != nil || result == nil {
		return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusError, LatencyMs: time.Since(started).Milliseconds(), Error: shortHealthError(fmt.Sprintf("route match failed: %v", err))}
	}
	var matched *router.MatchedRoute
	for _, candidate := range result.Routes {
		if candidate != nil && candidate.Route != nil && candidate.Provider != nil && candidate.ProviderAdapter != nil && candidate.Route.ID == target.RouteID && candidate.Provider.ID == target.ProviderID {
			matched = candidate
			break
		}
	}
	if matched == nil {
		return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusError, LatencyMs: time.Since(started).Milliseconds(), Error: "route/provider unavailable for model"}
	}
	mappedModel := e.mapModel(tenantID, target.Model, matched.Route, matched.Provider, domain.ClientTypeOpenAI, 0, apiTokenID)
	body := map[string]any{
		"model":      mappedModel,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
		"stream":     false,
	}
	requestBody, _ := json.Marshal(body)
	clientType := domain.ClientTypeOpenAI
	requestURI := "/v1/chat/completions"
	if !providerSupportsProbeType(matched.ProviderAdapter.SupportedClientTypes(), clientType) {
		targetType := GetPreferredTargetType(matched.ProviderAdapter.SupportedClientTypes(), clientType, matched.Provider.Type)
		if targetType != clientType {
			converted, convErr := e.converter.TransformRequest(clientType, targetType, requestBody, mappedModel, false)
			if convErr != nil {
				return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusError, LatencyMs: time.Since(started).Milliseconds(), Error: shortHealthError("request conversion failed: " + convErr.Error())}
			}
			requestBody = converted
			requestURI = ConvertRequestURI(requestURI, clientType, targetType, mappedModel, false)
			clientType = targetType
		}
	}
	req, _ := http.NewRequestWithContext(maxxctx.WithTenantID(ctx, tenantID), http.MethodPost, requestURI, bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := flow.NewCtx(NewResponseCapture(rec), req)
	c.Set(flow.KeyClientType, clientType)
	c.Set(flow.KeyOriginalClientType, domain.ClientTypeOpenAI)
	c.Set(flow.KeyRequestModel, target.Model)
	c.Set(flow.KeyMappedModel, mappedModel)
	c.Set(flow.KeyRequestBody, requestBody)
	c.Set(flow.KeyOriginalRequestBody, requestBody)
	c.Set(flow.KeyRequestHeaders, req.Header)
	c.Set(flow.KeyRequestURI, requestURI)
	c.Set(flow.KeyIsStream, false)
	c.Set(flow.KeyAPITokenID, apiTokenID)
	c.Set(flow.KeyProjectID, uint64(0))
	err = matched.ProviderAdapter.Execute(c, matched.Provider)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusError, LatencyMs: latency, Error: shortHealthError(err.Error())}
	}
	if rec.Code < 200 || rec.Code >= 300 {
		return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusError, LatencyMs: latency, Error: shortHealthError(fmt.Sprintf("status %d: %s", rec.Code, rec.Body.String()))}
	}
	return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusOK, LatencyMs: latency}
}

func providerSupportsProbeType(types []domain.ClientType, want domain.ClientType) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

func shortHealthError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 240 {
		return value[:240]
	}
	return value
}
