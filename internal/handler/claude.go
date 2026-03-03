package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/claude"
	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/service"
)

// ClaudeHandler handles Claude-specific API requests
type ClaudeHandler struct {
	svc          *service.AdminService
	oauthManager *claude.OAuthManager
	oauthServer  OAuthServer
}

// NewClaudeHandler creates a new Claude handler
func NewClaudeHandler(svc *service.AdminService, broadcaster event.Broadcaster) *ClaudeHandler {
	return &ClaudeHandler{
		svc:          svc,
		oauthManager: claude.NewOAuthManager(broadcaster),
	}
}

// SetOAuthServer injects the local OAuth callback server.
func (h *ClaudeHandler) SetOAuthServer(server OAuthServer) {
	h.oauthServer = server
}

// ServeHTTP routes Claude requests
// Routes:
//
//	POST /claude/validate-token - Validate refresh token
//	POST /claude/oauth/start - Start OAuth flow
//	GET  /claude/oauth/callback - OAuth callback
//	POST /claude/oauth/exchange - Manual callback URL exchange
//	POST /claude/provider/:id/refresh - Refresh provider info
func (h *ClaudeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/claude")
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")

	// POST /claude/validate-token
	if len(parts) >= 2 && parts[1] == "validate-token" && r.Method == http.MethodPost {
		h.handleValidateToken(w, r)
		return
	}

	// POST /claude/oauth/start
	if len(parts) >= 3 && parts[1] == "oauth" && parts[2] == "start" && r.Method == http.MethodPost {
		h.handleOAuthStart(w, r)
		return
	}

	// GET /claude/oauth/callback
	if len(parts) >= 3 && parts[1] == "oauth" && parts[2] == "callback" && r.Method == http.MethodGet {
		h.handleOAuthCallback(w, r)
		return
	}

	// POST /claude/oauth/exchange
	if len(parts) >= 3 && parts[1] == "oauth" && parts[2] == "exchange" && r.Method == http.MethodPost {
		h.handleOAuthExchange(w, r)
		return
	}

	// POST /claude/provider/:id/refresh
	if len(parts) >= 4 && parts[1] == "provider" && parts[3] == "refresh" && r.Method == http.MethodPost {
		h.handleRefreshProviderInfo(w, r, parts[2])
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

// ============================================================================
// Public methods (shared by HTTP handler and Wails)
// ============================================================================

// ValidateToken validates a refresh token
func (h *ClaudeHandler) ValidateToken(ctx context.Context, refreshToken string) (*claude.ClaudeTokenValidationResult, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("refreshToken is required")
	}

	return claude.ValidateRefreshToken(ctx, refreshToken)
}

// ClaudeOAuthStartResult OAuth start result
type ClaudeOAuthStartResult struct {
	AuthURL string `json:"authURL"`
	State   string `json:"state"`
}

// StartOAuth starts the OAuth authorization flow
func (h *ClaudeHandler) StartOAuth() (*ClaudeOAuthStartResult, error) {
	state, err := h.oauthManager.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	_, pkce, err := h.oauthManager.CreateSession(state)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	authURL := claude.GetAuthURL(state, pkce)

	return &ClaudeOAuthStartResult{
		AuthURL: authURL,
		State:   state,
	}, nil
}

// ============================================================================
// HTTP handler methods
// ============================================================================

func (h *ClaudeHandler) handleValidateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	result, err := h.ValidateToken(r.Context(), req.RefreshToken)
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *ClaudeHandler) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if h.oauthServer != nil && !h.oauthServer.IsRunning() {
		startCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		if err := h.oauthServer.Start(startCtx); err != nil {
			cancel()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		cancel()
	}

	result, err := h.StartOAuth()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *ClaudeHandler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.sendOAuthErrorResult(w, state, "Missing code or state parameter")
		return
	}

	session, ok := h.oauthManager.GetSession(state)
	if !ok {
		h.sendOAuthErrorResult(w, state, "Invalid or expired state")
		return
	}

	tokenResp, err := claude.ExchangeCodeForTokens(r.Context(), code, claude.OAuthRedirectURI, session.CodeVerifier)
	if err != nil {
		h.sendOAuthErrorResult(w, state, fmt.Sprintf("Token exchange failed: %v", err))
		return
	}

	var email, organizationID string
	if tokenResp.Account != nil {
		email = tokenResp.Account.EmailAddress
	}
	if tokenResp.Organization != nil {
		organizationID = tokenResp.Organization.UUID
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)

	result := &claude.OAuthResult{
		State:          state,
		Success:        true,
		AccessToken:    tokenResp.AccessToken,
		RefreshToken:   tokenResp.RefreshToken,
		ExpiresAt:      expiresAt,
		Email:          email,
		OrganizationID: organizationID,
	}

	h.oauthManager.CompleteSession(state, result)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(claudeOAuthSuccessHTML))

	h.stopOAuthServerAsync()
}

func (h *ClaudeHandler) handleOAuthExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if req.Code == "" || req.State == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing code or state parameter"})
		return
	}

	session, ok := h.oauthManager.GetSession(req.State)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid or expired state"})
		return
	}

	tokenResp, err := claude.ExchangeCodeForTokens(r.Context(), req.Code, claude.OAuthRedirectURI, session.CodeVerifier)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Token exchange failed: %v", err)})
		return
	}

	var email, organizationID string
	if tokenResp.Account != nil {
		email = tokenResp.Account.EmailAddress
	}
	if tokenResp.Organization != nil {
		organizationID = tokenResp.Organization.UUID
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)

	result := &claude.OAuthResult{
		State:          req.State,
		Success:        true,
		AccessToken:    tokenResp.AccessToken,
		RefreshToken:   tokenResp.RefreshToken,
		ExpiresAt:      expiresAt,
		Email:          email,
		OrganizationID: organizationID,
	}

	h.oauthManager.CompleteSession(req.State, result)

	writeJSON(w, http.StatusOK, result)
}

func (h *ClaudeHandler) sendOAuthErrorResult(w http.ResponseWriter, state, errorMsg string) {
	result := &claude.OAuthResult{
		State:   state,
		Success: false,
		Error:   errorMsg,
	}

	h.oauthManager.CompleteSession(state, result)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(claudeOAuthErrorHTML))

	h.stopOAuthServerAsync()
}

func (h *ClaudeHandler) stopOAuthServerAsync() {
	if h.oauthServer == nil || !h.oauthServer.IsRunning() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = h.oauthServer.Stop(ctx)
	}()
}

// RefreshProviderInfo refreshes the Claude provider info by re-validating the refresh token
func (h *ClaudeHandler) RefreshProviderInfo(ctx context.Context, providerID int) (*claude.ClaudeTokenValidationResult, error) {
	tenantID := maxxctx.GetTenantID(ctx)
	provider, err := h.svc.GetProvider(tenantID, uint64(providerID))
	if err != nil {
		return nil, fmt.Errorf("provider not found: %w", err)
	}

	if provider.Type != "claude" || provider.Config == nil || provider.Config.Claude == nil {
		return nil, fmt.Errorf("provider %s is not a claude provider", provider.Name)
	}

	refreshToken := provider.Config.Claude.RefreshToken
	if refreshToken == "" {
		return nil, fmt.Errorf("provider %s has no refresh token", provider.Name)
	}

	result, err := claude.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	if !result.Valid {
		return result, nil
	}

	// Update provider config with new info
	provider.Config.Claude.Email = result.Email
	provider.Config.Claude.AccessToken = result.AccessToken
	provider.Config.Claude.ExpiresAt = result.ExpiresAt
	provider.Config.Claude.OrganizationID = result.OrganizationID

	if result.RefreshToken != "" && result.RefreshToken != refreshToken {
		provider.Config.Claude.RefreshToken = result.RefreshToken
	}

	if err := h.svc.UpdateProvider(tenantID, provider); err != nil {
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	return result, nil
}

func (h *ClaudeHandler) handleRefreshProviderInfo(w http.ResponseWriter, r *http.Request, idStr string) {
	providerID, err := strconv.Atoi(idStr)
	if err != nil || providerID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid provider ID"})
		return
	}

	result, err := h.RefreshProviderInfo(r.Context(), providerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ============================================================================
// OAuth HTML pages
// ============================================================================

const claudeOAuthSuccessHTML = `<!DOCTYPE html>
<html>
<head><title>Authorization Successful</title></head>
<body style="font-family: sans-serif; text-align: center; padding: 60px;">
<h1>Authorization Successful</h1>
<p>You have been authorized with Claude. You may close this window.</p>
<p style="color: #666; font-size: 14px;">This window will close automatically in 10 seconds.</p>
<script>setTimeout(function() { window.close(); }, 10000);</script>
</body>
</html>`

const claudeOAuthErrorHTML = `<!DOCTYPE html>
<html>
<head><title>Authorization Failed</title></head>
<body style="font-family: sans-serif; text-align: center; padding: 60px;">
<h1>Authorization Failed</h1>
<p>There was an error during the authorization process. Please try again.</p>
<p style="color: #666; font-size: 14px;">This window will close automatically in 10 seconds.</p>
<script>setTimeout(function() { window.close(); }, 10000);</script>
</body>
</html>`
