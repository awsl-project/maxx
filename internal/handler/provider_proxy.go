package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/executor"
	"github.com/awsl-project/maxx/internal/executor/responsemodifier"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository"
)

// ProviderProxyHandler handles provider-prefixed proxy requests like /provider/{id}/v1/messages.
// Unlike the generic proxy path, provider-scoped requests are forwarded one-to-one to the
// requested provider without going through the generic route selection / retry chain.
type ProviderProxyHandler struct {
	proxyHandler     *ProxyHandler
	modelsHandler    *ModelsHandler
	providerRepo     repository.ProviderRepository
	routeRepo        repository.RouteRepository
	proxyRequestRepo repository.ProxyRequestRepository
}

// NewProviderProxyHandler creates a new provider proxy handler.
func NewProviderProxyHandler(
	proxyHandler *ProxyHandler,
	modelsHandler *ModelsHandler,
	providerRepo repository.ProviderRepository,
	routeRepo repository.RouteRepository,
	proxyRequestRepo repository.ProxyRequestRepository,
) *ProviderProxyHandler {
	return &ProviderProxyHandler{
		proxyHandler:     proxyHandler,
		modelsHandler:    modelsHandler,
		providerRepo:     providerRepo,
		routeRepo:        routeRepo,
		proxyRequestRepo: proxyRequestRepo,
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
		h.modelsHandler.ServeHTTP(w, r)
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

		route, err := h.routeRepo.FindByKey(tenantID, 0, provider.ID, clientType)
		if err != nil || route == nil {
			log.Printf("[ProviderProxy] route not found tenant=%d provider=%d clientType=%s: %v", tenantID, provider.ID, clientType, err)
			writeError(c.Writer, http.StatusNotFound, "provider route not found")
			c.Abort()
			return
		}

		factory, ok := provideradapter.GetAdapterFactory(provider.Type)
		if !ok {
			writeError(c.Writer, http.StatusBadGateway, "provider adapter not found")
			c.Abort()
			return
		}
		adapter, err := factory(provider)
		if err != nil {
			log.Printf("[ProviderProxy] failed to create adapter provider=%d type=%s: %v", provider.ID, provider.Type, err)
			writeError(c.Writer, http.StatusBadGateway, "provider adapter init failed")
			c.Abort()
			return
		}
		if !providerSupportsClientType(adapter.SupportedClientTypes(), clientType) {
			writeError(c.Writer, http.StatusBadRequest, "provider does not support this client type")
			c.Abort()
			return
		}

		requestModel := flow.GetRequestModel(c)
		mappedModel := requestModel
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

		clientWriter := http.ResponseWriter(c.Writer)
		modifierWriter := responsemodifier.NewResponseModifierWriter(c.Writer, provider, clientType, isStream)
		if modifierWriter != nil {
			clientWriter = modifierWriter
		}

		responseCapture := executor.NewResponseCapture(clientWriter)
		originalWriter := c.Writer
		c.Writer = responseCapture
		err = adapter.Execute(c, provider)
		c.Writer = originalWriter

		now := time.Now()
		proxyReq.EndTime = now
		proxyReq.Duration = now.Sub(proxyReq.StartTime)
		proxyReq.StatusCode = responseCapture.StatusCode()
		proxyReq.ResponseModel = mappedModel
		if !clearDetail {
			proxyReq.ResponseInfo = &domain.ResponseInfo{
				Status:  responseCapture.StatusCode(),
				Headers: responseCapture.CapturedHeaders(),
				Body:    responseCapture.Body(),
			}
		}

		if err == nil {
			if modifierWriter != nil {
				err = modifierWriter.Finalize()
				if err != nil {
					proxyReq.Status = "FAILED"
					proxyReq.Error = err.Error()
					clearProxyRequestDetail(proxyReq, clearDetail)
					_ = h.proxyRequestRepo.Update(proxyReq)
					c.Abort()
					return
				}
			}
			proxyReq.Status = "COMPLETED"
			clearProxyRequestDetail(proxyReq, clearDetail)
			_ = h.proxyRequestRepo.Update(proxyReq)
			return
		}

		proxyReq.Status = "FAILED"
		proxyReq.Error = err.Error()
		if proxyErr, ok := err.(*domain.ProxyError); ok {
			if isStream {
				writeStreamError(responseCapture, proxyErr)
			} else {
				writeProxyError(responseCapture, proxyErr)
			}
			if proxyErr.HTTPStatusCode >= 400 && proxyErr.HTTPStatusCode < 600 {
				proxyReq.StatusCode = proxyErr.HTTPStatusCode
			}
		} else {
			writeError(responseCapture, http.StatusBadGateway, err.Error())
			proxyReq.StatusCode = http.StatusBadGateway
		}
		if modifierWriter != nil {
			if finalizeErr := modifierWriter.Finalize(); finalizeErr != nil {
				log.Printf("[ProviderProxy] failed to finalize response modifier: %v", finalizeErr)
			}
		}
		clearProxyRequestDetail(proxyReq, clearDetail)
		_ = h.proxyRequestRepo.Update(proxyReq)
		c.Abort()
	}
}

func (h *ProviderProxyHandler) newProxyRequest(c *flow.Ctx, route *domain.Route, provider *domain.Provider, requestModel, mappedModel string, isStream bool, clearDetail bool) *domain.ProxyRequest {
	requestHeaders := flow.GetRequestHeaders(c)
	requestURI := flow.GetRequestURI(c)
	requestBody := flow.GetRequestBody(c)
	apiTokenID := flow.GetAPITokenID(c)
	projectID := flow.GetProjectID(c)
	tenantID := maxxctx.GetTenantID(c.Request.Context())
	devMode := getAPITokenDevMode(c)

	proxyReq := &domain.ProxyRequest{
		TenantID:      tenantID,
		RequestID:     generateProxyRequestID(),
		SessionID:     flow.GetSessionID(c),
		ClientType:    flow.GetClientType(c),
		RequestModel:  requestModel,
		ResponseModel: mappedModel,
		StartTime:     time.Now(),
		IsStream:      isStream,
		Status:        "IN_PROGRESS",
		StatusCode:    http.StatusOK,
		RouteID:       route.ID,
		ProviderID:    provider.ID,
		ProjectID:     projectID,
		APITokenID:    apiTokenID,
		DevMode:       devMode,
	}
	if !clearDetail {
		proxyReq.RequestInfo = &domain.RequestInfo{
			Method:  c.Request.Method,
			Headers: flattenRequestHeaders(requestHeaders),
			URL:     requestURI,
			Body:    string(requestBody),
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

func isValidProviderAPIPath(path string) bool {
	if path == "/v1/messages" || strings.HasPrefix(path, "/v1/messages/") {
		return true
	}
	if path == "/v1/chat/completions" || strings.HasPrefix(path, "/v1/chat/completions/") {
		return true
	}
	// OpenAI Images API (gpt-image-* generation + edits). Mirror isValidAPIPath
	// and proxy_routes.go: allow exactly the two registered endpoints rather
	// than HasPrefix("/v1/images/"), so the provider-prefixed contract doesn't
	// drift wider than the root.
	if path == "/v1/images/generations" || path == "/v1/images/edits" {
		return true
	}
	if path == "/responses" || strings.HasPrefix(path, "/responses/") {
		return true
	}
	if path == "/v1/responses" || strings.HasPrefix(path, "/v1/responses/") {
		return true
	}
	if path == "/v1/models" || strings.HasPrefix(path, "/v1/models/") {
		return true
	}
	if path == "/v1beta/models" || strings.HasPrefix(path, "/v1beta/models/") {
		return true
	}
	return false
}
