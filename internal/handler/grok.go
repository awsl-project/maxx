package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

const (
	grokOAuthClientID             = "b1a00492" + "-073a-47ea-816f-4c329264a828"
	grokOAuthScope                = "openid profile email offline_access grok-cli:access api:access"
	grokOAuthDeviceGrantType      = "urn:ietf:params:oauth:grant-type:device_code"
	grokOAuthDefaultBaseURL       = "https://api.x.ai/v1"
	grokOAuthDefaultPollSeconds   = 5
	grokOAuthHTTPTimeout          = 30 * time.Second
	grokOAuthMaxSessionLifetime   = 30 * time.Minute
	grokOAuthSessionIDRandomBytes = 24
)

var grokOAuthDiscoveryURL = "https://auth.x.ai/.well-known/openid-configuration"

type grokOAuthHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type GrokHandler struct {
	httpClient grokOAuthHTTPClient
	sessions   sync.Map // sessionID -> *grokOAuthSession
}

type grokOAuthSession struct {
	DeviceCode    string
	TokenEndpoint string
	ExpiresAt     time.Time
	Interval      int
}

type grokOAuthStartResult struct {
	SessionID               string `json:"sessionID"`
	VerificationURI         string `json:"verificationURI"`
	VerificationURIComplete string `json:"verificationURIComplete"`
	UserCode                string `json:"userCode"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

type grokOAuthPollRequest struct {
	SessionID string `json:"sessionID"`
}

type grokOAuthPollResult struct {
	Status     string                     `json:"status"`
	RetryAfter int                        `json:"retryAfter,omitempty"`
	Config     *domain.ProviderConfigGrok `json:"config,omitempty"`
	Label      string                     `json:"label,omitempty"`
	Error      string                     `json:"error,omitempty"`
}

type grokOAuthDiscovery struct {
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
}

type grokOAuthDeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type grokOAuthTokenPayload struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
}

func NewGrokHandler() *GrokHandler {
	return &GrokHandler{httpClient: &http.Client{Timeout: grokOAuthHTTPTimeout}}
}

func (h *GrokHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/grok")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")

	if len(parts) >= 3 && parts[1] == "oauth" && parts[2] == "start" && r.Method == http.MethodPost {
		h.handleOAuthStart(w, r)
		return
	}
	if len(parts) >= 3 && parts[1] == "oauth" && parts[2] == "poll" && r.Method == http.MethodPost {
		h.handleOAuthPoll(w, r)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (h *GrokHandler) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	result, err := h.StartOAuth(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *GrokHandler) handleOAuthPoll(w http.ResponseWriter, r *http.Request) {
	var req grokOAuthPollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	result, err := h.PollOAuth(r.Context(), req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *GrokHandler) StartOAuth(ctx context.Context) (*grokOAuthStartResult, error) {
	discovery, err := h.discover(ctx)
	if err != nil {
		return nil, err
	}
	deviceCode, err := h.requestDeviceCode(ctx, discovery)
	if err != nil {
		return nil, err
	}
	sessionID, err := randomURLSafe(grokOAuthSessionIDRandomBytes)
	if err != nil {
		return nil, fmt.Errorf("grok oauth: create session id: %w", err)
	}
	expiresIn := deviceCode.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int(grokOAuthMaxSessionLifetime.Seconds())
	}
	interval := deviceCode.Interval
	if interval < grokOAuthDefaultPollSeconds {
		interval = grokOAuthDefaultPollSeconds
	}
	h.sessions.Store(sessionID, &grokOAuthSession{
		DeviceCode:    deviceCode.DeviceCode,
		TokenEndpoint: discovery.TokenEndpoint,
		ExpiresAt:     time.Now().Add(time.Duration(expiresIn) * time.Second),
		Interval:      interval,
	})
	return &grokOAuthStartResult{
		SessionID:               sessionID,
		VerificationURI:         deviceCode.VerificationURI,
		VerificationURIComplete: deviceCode.VerificationURIComplete,
		UserCode:                deviceCode.UserCode,
		ExpiresIn:               expiresIn,
		Interval:                interval,
	}, nil
}

func (h *GrokHandler) PollOAuth(ctx context.Context, sessionID string) (*grokOAuthPollResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID is required")
	}
	value, ok := h.sessions.Load(sessionID)
	if !ok {
		return nil, fmt.Errorf("invalid or expired Grok OAuth session")
	}
	session, ok := value.(*grokOAuthSession)
	if !ok || session == nil || time.Now().After(session.ExpiresAt) {
		h.sessions.Delete(sessionID)
		return nil, fmt.Errorf("Grok OAuth session expired")
	}
	token, nextInterval, pending, err := h.exchangeDeviceCode(ctx, session)
	if pending {
		session.Interval = nextInterval
		h.sessions.Store(sessionID, session)
		return &grokOAuthPollResult{Status: "pending", RetryAfter: nextInterval}, nil
	}
	if err != nil {
		return nil, err
	}
	h.sessions.Delete(sessionID)
	email, subject := parseGrokIDTokenIdentity(token.IDToken)
	expiresAt := ""
	if token.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	config := &domain.ProviderConfigGrok{
		Type:          "xai",
		AuthKind:      "oauth",
		Email:         email,
		Sub:           subject,
		AccessToken:   strings.TrimSpace(token.AccessToken),
		RefreshToken:  strings.TrimSpace(token.RefreshToken),
		IDToken:       strings.TrimSpace(token.IDToken),
		TokenType:     strings.TrimSpace(token.TokenType),
		ExpiresIn:     token.ExpiresIn,
		Expired:       expiresAt,
		LastRefresh:   time.Now().UTC().Format(time.RFC3339),
		TokenEndpoint: session.TokenEndpoint,
		BaseURL:       grokOAuthDefaultBaseURL,
	}
	label := email
	if strings.TrimSpace(label) == "" {
		label = "xAI"
	}
	return &grokOAuthPollResult{Status: "authorized", Config: config, Label: label}, nil
}

func (h *GrokHandler) discover(ctx context.Context) (*grokOAuthDiscovery, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, grokOAuthDiscoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("grok oauth discovery: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok oauth discovery request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grok oauth discovery: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("grok oauth discovery failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var discovery grokOAuthDiscovery
	if err = json.Unmarshal(body, &discovery); err != nil {
		return nil, fmt.Errorf("grok oauth discovery: parse response: %w", err)
	}
	if discovery.DeviceAuthorizationEndpoint, err = validateGrokOAuthEndpoint(discovery.DeviceAuthorizationEndpoint, "device_authorization_endpoint"); err != nil {
		return nil, err
	}
	if discovery.TokenEndpoint, err = validateGrokOAuthEndpoint(discovery.TokenEndpoint, "token_endpoint"); err != nil {
		return nil, err
	}
	return &discovery, nil
}

func (h *GrokHandler) requestDeviceCode(ctx context.Context, discovery *grokOAuthDiscovery) (*grokOAuthDeviceCode, error) {
	form := url.Values{"client_id": {grokOAuthClientID}, "scope": {grokOAuthScope}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discovery.DeviceAuthorizationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("grok oauth device code: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok oauth device code request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grok oauth device code: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("grok oauth device code failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var deviceCode grokOAuthDeviceCode
	if err = json.Unmarshal(body, &deviceCode); err != nil {
		return nil, fmt.Errorf("grok oauth device code: parse response: %w", err)
	}
	if strings.TrimSpace(deviceCode.DeviceCode) == "" || strings.TrimSpace(deviceCode.UserCode) == "" {
		return nil, fmt.Errorf("grok oauth device code response missing required fields")
	}
	if strings.TrimSpace(deviceCode.VerificationURI) == "" && strings.TrimSpace(deviceCode.VerificationURIComplete) == "" {
		return nil, fmt.Errorf("grok oauth device code response missing verification URI")
	}
	return &deviceCode, nil
}

func (h *GrokHandler) exchangeDeviceCode(ctx context.Context, session *grokOAuthSession) (*grokOAuthTokenPayload, int, bool, error) {
	interval := session.Interval
	if interval < grokOAuthDefaultPollSeconds {
		interval = grokOAuthDefaultPollSeconds
	}
	form := url.Values{
		"grant_type":  {grokOAuthDeviceGrantType},
		"device_code": {strings.TrimSpace(session.DeviceCode)},
		"client_id":   {grokOAuthClientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, session.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, interval, false, fmt.Errorf("grok oauth token: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, interval, false, fmt.Errorf("grok oauth token request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, interval, false, fmt.Errorf("grok oauth token: read response: %w", err)
	}
	var payload grokOAuthTokenPayload
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, interval, false, fmt.Errorf("grok oauth token: parse response: %w", err)
	}
	if payload.Error != "" {
		switch payload.Error {
		case "authorization_pending":
			return nil, interval, true, nil
		case "slow_down":
			return nil, interval + grokOAuthDefaultPollSeconds, true, nil
		case "expired_token":
			return nil, interval, false, fmt.Errorf("grok oauth device code expired")
		case "access_denied":
			return nil, interval, false, fmt.Errorf("grok oauth authorization denied")
		default:
			desc := strings.TrimSpace(payload.ErrorDescription)
			if desc != "" {
				return nil, interval, false, fmt.Errorf("grok oauth token error: %s: %s", payload.Error, desc)
			}
			return nil, interval, false, fmt.Errorf("grok oauth token error: %s", payload.Error)
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, interval, false, fmt.Errorf("grok oauth token failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return nil, interval, false, fmt.Errorf("grok oauth token response missing access_token")
	}
	return &payload, interval, false, nil
}

func validateGrokOAuthEndpoint(rawURL string, field string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("grok oauth discovery %s is empty", field)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("grok oauth discovery %s is invalid: %w", field, err)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("grok oauth discovery %s must use https: %q", field, rawURL)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host != "x.ai" && !strings.HasSuffix(host, ".x.ai") {
		return "", fmt.Errorf("grok oauth discovery %s host %q is not on x.ai", field, host)
	}
	return rawURL, nil
}

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func parseGrokIDTokenIdentity(token string) (email string, subject string) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", ""
	}
	payload := parts[1]
	payload += strings.Repeat("=", (4-len(payload)%4)%4)
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return "", ""
	}
	var claims map[string]any
	if err = json.Unmarshal(raw, &claims); err != nil {
		return "", ""
	}
	if v, ok := claims["email"].(string); ok {
		email = strings.TrimSpace(v)
	}
	if v, ok := claims["sub"].(string); ok {
		subject = strings.TrimSpace(v)
	}
	return email, subject
}
