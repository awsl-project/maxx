package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/service"
)

// AdminHandler handles admin API requests over HTTP
// Delegates business logic to AdminService
type AdminHandler struct {
	svc     *service.AdminService
	logPath string
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(svc *service.AdminService, logPath string) *AdminHandler {
	return &AdminHandler{
		svc:     svc,
		logPath: logPath,
	}
}

// ServeHTTP routes admin requests
func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin")
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	resource := parts[1]
	var id uint64
	if len(parts) > 2 && parts[2] != "" {
		id, _ = strconv.ParseUint(parts[2], 10, 64)
	}

	switch resource {
	case "providers":
		h.handleProviders(w, r, id)
	case "routes":
		if len(parts) > 2 && parts[2] == "batch-positions" {
			h.handleBatchUpdateRoutePositions(w, r)
		} else {
			h.handleRoutes(w, r, id)
		}
	case "projects":
		h.handleProjects(w, r, id, parts)
	case "sessions":
		h.handleSessions(w, r, parts)
	case "retry-configs":
		h.handleRetryConfigs(w, r, id)
	case "routing-strategies":
		h.handleRoutingStrategies(w, r, id)
	case "requests":
		h.handleProxyRequests(w, r, id, parts)
	case "settings":
		h.handleSettings(w, r, parts)
	case "proxy-status":
		h.handleProxyStatus(w, r)
	case "provider-stats":
		h.handleProviderStats(w, r)
	case "cooldowns":
		h.handleCooldowns(w, r, id)
	case "logs":
		h.handleLogs(w, r)
	case "api-tokens":
		h.handleAPITokens(w, r, id)
	case "model-mappings":
		h.handleModelMappings(w, r, id)
	case "usage-stats":
		h.handleUsageStats(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// Provider handlers
func (h *AdminHandler) handleProviders(w http.ResponseWriter, r *http.Request, id uint64) {
	// Check for special endpoints
	path := r.URL.Path
	if strings.HasSuffix(path, "/export") {
		h.handleProvidersExport(w, r)
		return
	}
	if strings.HasSuffix(path, "/import") {
		h.handleProvidersImport(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			provider, err := h.svc.GetProvider(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
				return
			}
			writeJSON(w, http.StatusOK, provider)
		} else {
			providers, err := h.svc.GetProviders()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, providers)
		}
	case http.MethodPost:
		var provider domain.Provider
		if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.CreateProvider(&provider); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, provider)
	case http.MethodPut:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		// Get existing provider first for merge update
		existing, err := h.svc.GetProvider(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
			return
		}
		// Decode the update - for Provider, we expect full object updates from the form,
		// but we still need to preserve ID and timestamps
		var provider domain.Provider
		if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		// Preserve ID and timestamps
		provider.ID = existing.ID
		provider.CreatedAt = existing.CreatedAt
		if err := h.svc.UpdateProvider(&provider); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, provider)
	case http.MethodDelete:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteProvider(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleProvidersExport exports all providers as JSON
func (h *AdminHandler) handleProvidersExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	providers, err := h.svc.ExportProviders()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=providers.json")
	json.NewEncoder(w).Encode(providers)
}

// handleProvidersImport imports providers from JSON
func (h *AdminHandler) handleProvidersImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var providers []*domain.Provider
	if err := json.NewDecoder(r.Body).Decode(&providers); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	result, err := h.svc.ImportProviders(providers)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// Route handlers
func (h *AdminHandler) handleRoutes(w http.ResponseWriter, r *http.Request, id uint64) {
	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			route, err := h.svc.GetRoute(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
				return
			}
			writeJSON(w, http.StatusOK, route)
		} else {
			routes, err := h.svc.GetRoutes()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, routes)
		}
	case http.MethodPost:
		var route domain.Route
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.CreateRoute(&route); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, route)
	case http.MethodPut:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		// Get existing route first for merge update
		existing, err := h.svc.GetRoute(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
			return
		}
		// Decode partial update into a map to detect which fields were sent
		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		// Apply updates to existing route (with safe type assertions)
		if v, ok := updates["isEnabled"]; ok {
			if b, ok := v.(bool); ok {
				existing.IsEnabled = b
			}
		}
		if v, ok := updates["isNative"]; ok {
			if b, ok := v.(bool); ok {
				existing.IsNative = b
			}
		}
		if v, ok := updates["projectID"]; ok {
			if f, ok := v.(float64); ok {
				existing.ProjectID = uint64(f)
			}
		}
		if v, ok := updates["clientType"]; ok {
			if s, ok := v.(string); ok {
				existing.ClientType = domain.ClientType(s)
			}
		}
		if v, ok := updates["providerID"]; ok {
			if f, ok := v.(float64); ok {
				existing.ProviderID = uint64(f)
			}
		}
		if v, ok := updates["position"]; ok {
			if f, ok := v.(float64); ok {
				existing.Position = int(f)
			}
		}
		if v, ok := updates["retryConfigID"]; ok {
			if f, ok := v.(float64); ok {
				existing.RetryConfigID = uint64(f)
			}
		}
		if v, ok := updates["modelMapping"]; ok {
			if v == nil {
				existing.ModelMapping = nil
			} else if m, ok := v.(map[string]interface{}); ok {
				existing.ModelMapping = make(map[string]string)
				for k, val := range m {
					if s, ok := val.(string); ok {
						existing.ModelMapping[k] = s
					}
				}
			}
		}
		if err := h.svc.UpdateRoute(existing); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, existing)
	case http.MethodDelete:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteRoute(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// Batch update route positions
func (h *AdminHandler) handleBatchUpdateRoutePositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var updates []domain.RoutePositionUpdate
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.svc.BatchUpdateRoutePositions(updates); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "positions updated successfully"})
}

// Project handlers
func (h *AdminHandler) handleProjects(w http.ResponseWriter, r *http.Request, id uint64, parts []string) {
	// Check for by-slug endpoint: /admin/projects/by-slug/{slug}
	if len(parts) > 2 && parts[2] == "by-slug" {
		h.handleProjectBySlug(w, r, parts)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			project, err := h.svc.GetProject(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
				return
			}
			writeJSON(w, http.StatusOK, project)
		} else {
			projects, err := h.svc.GetProjects()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, projects)
		}
	case http.MethodPost:
		var project domain.Project
		if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.CreateProject(&project); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, project)
	case http.MethodPut:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		// Get existing project first to preserve timestamps
		existing, err := h.svc.GetProject(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}
		var project domain.Project
		if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		project.ID = existing.ID
		project.CreatedAt = existing.CreatedAt
		if err := h.svc.UpdateProject(&project); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, project)
	case http.MethodDelete:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteProject(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleProjectBySlug handles GET /admin/projects/by-slug/{slug}
func (h *AdminHandler) handleProjectBySlug(w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if len(parts) < 4 || parts[3] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug required"})
		return
	}

	slug := parts[3]
	project, err := h.svc.GetProjectBySlug(slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	writeJSON(w, http.StatusOK, project)
}

// Session handlers
// Routes: /admin/sessions, /admin/sessions/{sessionID}/project, /admin/sessions/{sessionID}/reject
func (h *AdminHandler) handleSessions(w http.ResponseWriter, r *http.Request, parts []string) {
	// Check for sub-resource: /admin/sessions/{sessionID}/project
	if len(parts) > 3 && parts[3] == "project" {
		h.handleSessionProject(w, r, parts[2])
		return
	}

	// Check for sub-resource: /admin/sessions/{sessionID}/reject
	if len(parts) > 3 && parts[3] == "reject" {
		h.handleSessionReject(w, r, parts[2])
		return
	}

	switch r.Method {
	case http.MethodGet:
		sessions, err := h.svc.GetSessions()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, sessions)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleSessionProject handles PUT /admin/sessions/{sessionID}/project
func (h *AdminHandler) handleSessionProject(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session ID required"})
		return
	}

	var body struct {
		ProjectID uint64 `json:"projectID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	result, err := h.svc.UpdateSessionProject(sessionID, body.ProjectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleSessionReject handles POST /admin/sessions/{sessionID}/reject
func (h *AdminHandler) handleSessionReject(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session ID required"})
		return
	}

	session, err := h.svc.RejectSession(sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, session)
}

// RetryConfig handlers
func (h *AdminHandler) handleRetryConfigs(w http.ResponseWriter, r *http.Request, id uint64) {
	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			config, err := h.svc.GetRetryConfig(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "retry config not found"})
				return
			}
			writeJSON(w, http.StatusOK, config)
		} else {
			configs, err := h.svc.GetRetryConfigs()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, configs)
		}
	case http.MethodPost:
		var config domain.RetryConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.CreateRetryConfig(&config); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, config)
	case http.MethodPut:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		// Get existing config first to preserve timestamps
		existing, err := h.svc.GetRetryConfig(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "retry config not found"})
			return
		}
		var config domain.RetryConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		config.ID = existing.ID
		config.CreatedAt = existing.CreatedAt
		if err := h.svc.UpdateRetryConfig(&config); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, config)
	case http.MethodDelete:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteRetryConfig(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// RoutingStrategy handlers
func (h *AdminHandler) handleRoutingStrategies(w http.ResponseWriter, r *http.Request, id uint64) {
	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			strategy, err := h.svc.GetRoutingStrategy(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "routing strategy not found"})
				return
			}
			writeJSON(w, http.StatusOK, strategy)
		} else {
			strategies, err := h.svc.GetRoutingStrategies()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, strategies)
		}
	case http.MethodPost:
		var strategy domain.RoutingStrategy
		if err := json.NewDecoder(r.Body).Decode(&strategy); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.CreateRoutingStrategy(&strategy); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, strategy)
	case http.MethodPut:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		// Get existing strategy first to preserve timestamps
		existing, err := h.svc.GetRoutingStrategy(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "routing strategy not found"})
			return
		}
		var strategy domain.RoutingStrategy
		if err := json.NewDecoder(r.Body).Decode(&strategy); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		strategy.ID = existing.ID
		strategy.CreatedAt = existing.CreatedAt
		if err := h.svc.UpdateRoutingStrategy(&strategy); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, strategy)
	case http.MethodDelete:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteRoutingStrategy(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ProxyRequest handlers
// Routes: /admin/requests, /admin/requests/count, /admin/requests/{id}, /admin/requests/{id}/attempts
func (h *AdminHandler) handleProxyRequests(w http.ResponseWriter, r *http.Request, id uint64, parts []string) {
	// Check for count endpoint: /admin/requests/count
	if len(parts) > 2 && parts[2] == "count" {
		h.handleProxyRequestsCount(w, r)
		return
	}

	// Check for sub-resource: /admin/requests/{id}/attempts
	if len(parts) > 3 && parts[3] == "attempts" && id > 0 {
		h.handleProxyUpstreamAttempts(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			req, err := h.svc.GetProxyRequest(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "proxy request not found"})
				return
			}
			writeJSON(w, http.StatusOK, req)
		} else {
			limit := 100
			var before, after uint64
			if l := r.URL.Query().Get("limit"); l != "" {
				limit, _ = strconv.Atoi(l)
			}
			if b := r.URL.Query().Get("before"); b != "" {
				before, _ = strconv.ParseUint(b, 10, 64)
			}
			if a := r.URL.Query().Get("after"); a != "" {
				after, _ = strconv.ParseUint(a, 10, 64)
			}
			result, err := h.svc.GetProxyRequestsCursor(limit, before, after)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, result)
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ProxyRequestsCount handler
func (h *AdminHandler) handleProxyRequestsCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	count, err := h.svc.GetProxyRequestsCount()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, count)
}

// ProxyUpstreamAttempt handlers
func (h *AdminHandler) handleProxyUpstreamAttempts(w http.ResponseWriter, r *http.Request, proxyRequestID uint64) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	attempts, err := h.svc.GetProxyUpstreamAttempts(proxyRequestID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, attempts)
}

// Settings handlers
func (h *AdminHandler) handleSettings(w http.ResponseWriter, r *http.Request, parts []string) {
	var key string
	if len(parts) > 2 {
		key = parts[2]
	}

	switch r.Method {
	case http.MethodGet:
		if key != "" {
			value, err := h.svc.GetSetting(key)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
		} else {
			settings, err := h.svc.GetSettings()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, settings)
		}
	case http.MethodPut, http.MethodPost:
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key required"})
			return
		}
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.UpdateSetting(key, body.Value); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": body.Value})
	case http.MethodDelete:
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key required"})
			return
		}
		if err := h.svc.DeleteSetting(key); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// Proxy status handler
func (h *AdminHandler) handleProxyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, h.svc.GetProxyStatus(r))
}

// Provider stats handler
func (h *AdminHandler) handleProviderStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	clientType := r.URL.Query().Get("client_type")
	var projectID uint64
	if pidStr := r.URL.Query().Get("project_id"); pidStr != "" {
		projectID, _ = strconv.ParseUint(pidStr, 10, 64)
	}
	stats, err := h.svc.GetProviderStats(clientType, projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// Logs handler
func (h *AdminHandler) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	lines, err := ReadLastNLines(h.logPath, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lines": lines,
		"count": len(lines),
	})
}

// Cooldowns handler
// GET /admin/cooldowns - list all active cooldowns
// DELETE /admin/cooldowns/{id} - clear cooldown for a provider
func (h *AdminHandler) handleCooldowns(w http.ResponseWriter, r *http.Request, providerID uint64) {
	cm := cooldown.Default()

	switch r.Method {
	case http.MethodGet:
		// Get all active cooldowns
		cooldowns := cm.GetAllCooldowns()
		providers, _ := h.svc.GetProviders()

		// Build provider name map
		providerNames := make(map[uint64]string)
		for _, p := range providers {
			providerNames[p.ID] = p.Name
		}

		// Build response using GetCooldownInfo to include reason
		var result []*cooldown.CooldownInfo
		for key := range cooldowns {
			info := cm.GetCooldownInfo(key.ProviderID, key.ClientType, providerNames[key.ProviderID])
			if info != nil {
				result = append(result, info)
			}
		}

		writeJSON(w, http.StatusOK, result)

	case http.MethodDelete:
		if providerID == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider id required"})
			return
		}
		// Clear all cooldowns for this provider (both global and client-type-specific)
		cm.ClearCooldown(providerID, "")
		writeJSON(w, http.StatusOK, map[string]string{"message": "cooldown cleared"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// API Token handlers
func (h *AdminHandler) handleAPITokens(w http.ResponseWriter, r *http.Request, id uint64) {
	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			token, err := h.svc.GetAPIToken(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
				return
			}
			writeJSON(w, http.StatusOK, token)
		} else {
			tokens, err := h.svc.GetAPITokens()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, tokens)
		}
	case http.MethodPost:
		var body struct {
			Name        string  `json:"name"`
			Description string  `json:"description"`
			ProjectID   uint64  `json:"projectID"`
			ExpiresAt   *string `json:"expiresAt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if body.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		var expiresAt *time.Time
		if body.ExpiresAt != nil && *body.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid expiresAt format, use RFC3339"})
				return
			}
			expiresAt = &t
		}
		result, err := h.svc.CreateAPIToken(body.Name, body.Description, body.ProjectID, expiresAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, result)
	case http.MethodPut:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		existing, err := h.svc.GetAPIToken(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
			return
		}
		var body struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			ProjectID   *uint64 `json:"projectID"`
			IsEnabled   *bool   `json:"isEnabled"`
			ExpiresAt   *string `json:"expiresAt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if body.Name != nil {
			if *body.Name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
				return
			}
			existing.Name = *body.Name
		}
		if body.Description != nil {
			existing.Description = *body.Description
		}
		if body.ProjectID != nil {
			existing.ProjectID = *body.ProjectID
		}
		if body.IsEnabled != nil {
			existing.IsEnabled = *body.IsEnabled
		}
		if body.ExpiresAt != nil {
			if *body.ExpiresAt == "" {
				existing.ExpiresAt = nil
			} else {
				t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid expiresAt format, use RFC3339"})
					return
				}
				existing.ExpiresAt = &t
			}
		}
		if err := h.svc.UpdateAPIToken(existing); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, existing)
	case http.MethodDelete:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteAPIToken(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// Model Mapping handlers
func (h *AdminHandler) handleModelMappings(w http.ResponseWriter, r *http.Request, id uint64) {
	// Check for clear-all endpoint: /admin/model-mappings/clear-all
	path := r.URL.Path
	if strings.HasSuffix(path, "/clear-all") {
		h.handleClearAllModelMappings(w, r)
		return
	}
	// Check for reset-defaults endpoint: /admin/model-mappings/reset-defaults
	if strings.HasSuffix(path, "/reset-defaults") {
		h.handleResetModelMappingsToDefaults(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			mapping, err := h.svc.GetModelMapping(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "mapping not found"})
				return
			}
			writeJSON(w, http.StatusOK, mapping)
		} else {
			mappings, err := h.svc.GetModelMappings()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, mappings)
		}
	case http.MethodPost:
		var mapping domain.ModelMapping
		if err := json.NewDecoder(r.Body).Decode(&mapping); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if mapping.Pattern == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pattern is required"})
			return
		}
		if mapping.Target == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target is required"})
			return
		}
		if err := h.svc.CreateModelMapping(&mapping); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, mapping)
	case http.MethodPut:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		existing, err := h.svc.GetModelMapping(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "mapping not found"})
			return
		}
		var body struct {
			ClientType *string `json:"clientType"`
			Pattern    *string `json:"pattern"`
			Target     *string `json:"target"`
			Priority   *int    `json:"priority"`
			IsEnabled  *bool   `json:"isEnabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if body.ClientType != nil {
			existing.ClientType = domain.ClientType(*body.ClientType)
		}
		if body.Pattern != nil {
			if *body.Pattern == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pattern cannot be empty"})
				return
			}
			existing.Pattern = *body.Pattern
		}
		if body.Target != nil {
			if *body.Target == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target cannot be empty"})
				return
			}
			existing.Target = *body.Target
		}
		if body.Priority != nil {
			existing.Priority = *body.Priority
		}
		if body.IsEnabled != nil {
			existing.IsEnabled = *body.IsEnabled
		}
		if err := h.svc.UpdateModelMapping(existing); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, existing)
	case http.MethodDelete:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteModelMapping(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleClearAllModelMappings handles DELETE /admin/model-mappings/clear-all
func (h *AdminHandler) handleClearAllModelMappings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if err := h.svc.ClearAllModelMappings(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "all mappings cleared"})
}

// handleResetModelMappingsToDefaults handles POST /admin/model-mappings/reset-defaults
func (h *AdminHandler) handleResetModelMappingsToDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if err := h.svc.ResetModelMappingsToDefaults(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "mappings reset to defaults"})
}

// Usage Stats handlers
func (h *AdminHandler) handleUsageStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Parse query parameters for filtering
	query := r.URL.Query()
	filter := repository.UsageStatsFilter{}

	// Parse time range (转换到本地时区)
	if startStr := query.Get("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			local := t.Local()
			filter.StartTime = &local
		}
	}
	if endStr := query.Get("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			local := t.Local()
			filter.EndTime = &local
		}
	}

	// Parse IDs
	if routeIDStr := query.Get("routeId"); routeIDStr != "" {
		if id, err := strconv.ParseUint(routeIDStr, 10, 64); err == nil {
			filter.RouteID = &id
		}
	}
	if providerIDStr := query.Get("providerId"); providerIDStr != "" {
		if id, err := strconv.ParseUint(providerIDStr, 10, 64); err == nil {
			filter.ProviderID = &id
		}
	}
	if projectIDStr := query.Get("projectId"); projectIDStr != "" {
		if id, err := strconv.ParseUint(projectIDStr, 10, 64); err == nil {
			filter.ProjectID = &id
		}
	}
	if clientType := query.Get("clientType"); clientType != "" {
		filter.ClientType = &clientType
	}
	if apiTokenIDStr := query.Get("apiTokenId"); apiTokenIDStr != "" {
		if id, err := strconv.ParseUint(apiTokenIDStr, 10, 64); err == nil {
			filter.APITokenID = &id
		}
	}

	stats, err := h.svc.GetUsageStats(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}
