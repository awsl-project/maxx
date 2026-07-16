package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/requestmeta"
)

// ProviderProxyHandler handles provider-prefixed proxy requests like /provider/{id}/v1/messages.
// Unlike the generic proxy path, provider-scoped requests are forwarded one-to-one to the
// requested provider without going through the generic route selection / retry chain.
type ProviderProxyHandler struct {
	proxyHandler     *ProxyHandler
	modelsHandler    http.Handler
	providerRepo     repository.ProviderRepository
	routeRepo        repository.RouteRepository
	proxyRequestRepo repository.ProxyRequestRepository
	settingsRepo     repository.SystemSettingRepository
}

// NewProviderProxyHandler creates a new provider proxy handler.
func NewProviderProxyHandler(
	proxyHandler *ProxyHandler,
	modelsHandler http.Handler,
	providerRepo repository.ProviderRepository,
	routeRepo repository.RouteRepository,
	proxyRequestRepo repository.ProxyRequestRepository,
	settingsRepo repository.SystemSettingRepository,
) *ProviderProxyHandler {
	return &ProviderProxyHandler{
		proxyHandler:     proxyHandler,
		modelsHandler:    modelsHandler,
		providerRepo:     providerRepo,
		routeRepo:        routeRepo,
		proxyRequestRepo: proxyRequestRepo,
		settingsRepo:     settingsRepo,
	}
}

// ServeHTTP handles provider-prefixed proxy requests.
func (h *ProviderProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	providerID, apiPath, ok := h.parseProviderPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "invalid provider proxy path")
		return
	}

	providerIDNum, err := strconv.ParseUint(providerID, 10, 64)
	if err != nil || providerIDNum == 0 {
		writeError(w, http.StatusBadRequest, "invalid provider id")
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	provider, err := h.providerRepo.GetByID(tenantID, providerIDNum)
	if err != nil {
		log.Printf("[ProviderProxy] failed to load provider tenant=%d id=%d: %v", tenantID, providerIDNum, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if provider == nil {
		log.Printf("[ProviderProxy] Provider not found for id: %s", providerID)
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}

	if isModelListAPIPath(apiPath) {
		r.URL.Path = apiPath
		r.Header.Set("X-Maxx-Provider-ID", strconv.FormatUint(provider.ID, 10))
		h.modelsHandler.ServeHTTP(w, r)
		return
	}
	if !proxyRouteExposureEnabled(h.settingsRepo, apiPath) {
		http.NotFound(w, r)
		return
	}

	log.Printf("[ProviderProxy] Direct forwarding through provider: %s (ID: %d)", provider.Name, provider.ID)
	r.URL.Path = apiPath

	ctx := flow.NewCtx(w, r)
	handlers := append([]flow.HandlerFunc{}, h.proxyHandler.extra...)
	handlers = append(handlers, h.directDispatch(provider))
	h.proxyHandler.engine.HandleWith(ctx, handlers...)
}

func (h *ProviderProxyHandler) directDispatch(provider *domain.Provider) flow.HandlerFunc {
	return func(c *flow.Ctx) {
		tenantID := maxxctx.GetTenantID(c.Request.Context())
		clientType := flow.GetClientType(c)
		if clientType == "" {
			writeError(c.Writer, http.StatusBadRequest, "unable to determine client type")
			c.Abort()
			return
		}

		requestModel := flow.GetRequestModel(c)
		projectID := flow.GetProjectID(c)
		apiTokenID := flow.GetAPITokenID(c)
		sessionID := flow.GetSessionID(c)
		if h.proxyHandler == nil || h.proxyHandler.executor == nil {
			writeError(c.Writer, http.StatusInternalServerError, "provider proxy executor not configured")
			c.Abort()
			return
		}
		matchedRoute, err := h.proxyHandler.executor.MatchProviderProxyRoute(c.Request.Context(), tenantID, provider.ID, clientType, projectID, requestModel, apiTokenID, sessionID)
		if err != nil || matchedRoute == nil || matchedRoute.Route == nil || matchedRoute.Provider == nil || matchedRoute.ProviderAdapter == nil {
			log.Printf("[ProviderProxy] matched route not found tenant=%d project=%d provider=%d clientType=%s model=%s: %v", tenantID, projectID, provider.ID, clientType, requestModel, err)
			writeError(c.Writer, http.StatusNotFound, "provider route not found")
			c.Abort()
			return
		}
		route := matchedRoute.Route
		provider = matchedRoute.Provider
		mappedModel := requestModel
		mappedModel = h.proxyHandler.executor.MapModelForProviderProxy(tenantID, requestModel, route, provider, clientType, projectID, apiTokenID)
		isStream := flow.GetIsStream(c)
		clearDetail := h.proxyHandler != nil && h.proxyHandler.executor != nil && h.proxyHandler.executor.ShouldClearRequestDetailByConfig()
		if getAPITokenDevMode(c) {
			clearDetail = false
		}
		proxyReq := h.newProxyRequest(c, route, provider, requestModel, mappedModel, isStream, clearDetail)
		if err := h.proxyRequestRepo.Create(proxyReq); err != nil {
			log.Printf("[ProviderProxy] failed to create proxy request: %v", err)
		}

		c.Set(flow.KeyMappedModel, mappedModel)
		c.Set(flow.KeyOriginalClientType, clientType)
		c.Set(flow.KeyProxyRequest, proxyReq)

		// Route direct provider calls through the normal executor dispatch loop,
		// constrained to the provider selected above by the generic matcher.
		err = h.proxyHandler.executor.ExecuteProviderProxyMatched(c, proxyReq, matchedRoute)
		if err == nil {
			return
		}

		if proxyErr, ok := err.(*domain.ProxyError); ok {
			if isStream {
				writeStreamError(c.Writer, proxyErr)
			} else {
				writeProxyError(c.Writer, proxyErr)
			}
		} else {
			writeError(c.Writer, http.StatusBadGateway, err.Error())
		}
		c.Abort()
	}
}

func (h *ProviderProxyHandler) newProxyRequest(c *flow.Ctx, route *domain.Route, provider *domain.Provider, requestModel, mappedModel string, isStream bool, clearDetail bool) *domain.ProxyRequest {
	requestHeaders := flow.GetRequestHeaders(c)
	requestURI := flow.GetRequestURI(c)
	requestBody := flow.GetRequestBody(c)
	reasoningBody := flow.GetOriginalRequestBody(c)
	if len(reasoningBody) == 0 {
		reasoningBody = requestBody
	}
	apiTokenID := flow.GetAPITokenID(c)
	projectID := flow.GetProjectID(c)
	tenantID := maxxctx.GetTenantID(c.Request.Context())
	devMode := getAPITokenDevMode(c)

	proxyReq := &domain.ProxyRequest{
		TenantID:        tenantID,
		RequestID:       generateProxyRequestID(),
		SessionID:       flow.GetSessionID(c),
		ClientType:      flow.GetClientType(c),
		RequestModel:    requestModel,
		ResponseModel:   mappedModel,
		ReasoningEffort: requestmeta.ReasoningEffort(reasoningBody),
		StartTime:       time.Now(),
		IsStream:        isStream,
		Status:          "IN_PROGRESS",
		StatusCode:      http.StatusOK,
		RouteID:         route.ID,
		ProviderID:      provider.ID,
		ProjectID:       projectID,
		APITokenID:      apiTokenID,
		DevMode:         devMode,
	}
	if !clearDetail {
		proxyReq.RequestInfo = &domain.RequestInfo{
			Method:  c.Request.Method,
			Headers: flattenRequestHeaders(requestHeaders),
			URL:     requestURI,
			Body:    domain.RequestBodySnapshot(requestBody, requestHeaders.Get("Content-Type"), devMode),
		}
	}
	return proxyReq
}

func getAPITokenDevMode(c *flow.Ctx) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(flow.KeyAPITokenDevMode)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func clearProxyRequestDetail(req *domain.ProxyRequest, clearDetail bool) {
	if !clearDetail || req == nil {
		return
	}
	req.RequestInfo = nil
	req.ResponseInfo = nil
}

func generateProxyRequestID() string {
	return time.Now().Format("20060102150405.000000")
}

func flattenRequestHeaders(h http.Header) map[string]string {
	if h == nil {
		return nil
	}
	result := make(map[string]string)
	for key, values := range h {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

func providerSupportsClientType(supported []domain.ClientType, clientType domain.ClientType) bool {
	for _, ct := range supported {
		if ct == clientType {
			return true
		}
	}
	return false
}

// parseProviderPath extracts the provider ID and API path from a provider-prefixed URL.
func (h *ProviderProxyHandler) parseProviderPath(path string) (providerID, apiPath string, ok bool) {
	if !strings.HasPrefix(path, "/provider/") {
		return "", "", false
	}

	path = strings.TrimPrefix(path, "/provider/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return "", "", false
	}

	providerID = strings.TrimSpace(parts[0])
	if providerID == "" {
		return "", "", false
	}

	apiPath = "/" + parts[1]
	if !isValidProviderAPIPath(apiPath) {
		return "", "", false
	}
	return providerID, apiPath, true
}

func isProviderProxyPath(urlPath string) bool {
	return strings.HasPrefix(urlPath, "/provider/")
}

// isValidProviderAPIPath shares the proxyAPIEndpoints table with isValidAPIPath
// so the provider- and project-scoped allowlists stay identical by construction.
func isValidProviderAPIPath(path string) bool {
	return isValidProxyAPIPath(path)
}
