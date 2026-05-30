// Package api wraps the maxx admin HTTP API as typed Go calls.
package api

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/cfg"
	"github.com/awsl-project/maxx/internal/domain"
)

// maxResponseBytes caps how much we read from a single admin API response.
// Admin payloads are small (a few MiB at most for provider exports); this
// guard exists so a buggy or hostile server can't OOM the CLI.
const maxResponseBytes = 32 << 20 // 32 MiB

// insecureWarnedOnce tracks contexts we have already warned about so the
// stderr message fires once per process per context name, not on every
// command invocation.
var insecureWarnedOnce sync.Map // map[string]*sync.Once

// Client is the admin HTTP API client.
type Client struct {
	server string
	token  string
	http   *http.Client
}

// warnInsecure prints the InsecureSkipVerify reminder to stderr at most
// once per process per context name.
func warnInsecure(name string) {
	v, _ := insecureWarnedOnce.LoadOrStore(name, &sync.Once{})
	v.(*sync.Once).Do(func() {
		fmt.Fprintf(os.Stderr,
			"[maxx-cli] warning: context %q has insecureSkipVerify=true; TLS certificate verification is disabled\n",
			name)
	})
}

// NewFromContext builds a client from a CLI context.
func NewFromContext(ctx *cfg.Context) (*Client, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	u, err := url.Parse(ctx.Server)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL %q: %w", ctx.Server, err)
	}
	// url.Parse is permissive — "localhost:9880" parses as scheme=localhost
	// path=9880, which produces gibberish requests. Require explicit
	// http/https and a non-empty host so the error happens at config time.
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid server URL %q: scheme must be http or https", ctx.Server)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid server URL %q: missing host", ctx.Server)
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: ctx.InsecureSkipVerify},
	}
	if ctx.InsecureSkipVerify {
		// Make the security trade-off visible, but only once per process per
		// context — every-invocation noise becomes invisible and pollutes
		// pipelines (it would leak into shells that mix stderr into stdout).
		warnInsecure(ctx.Name)
	}
	return &Client{
		server: strings.TrimRight(ctx.Server, "/"),
		token:  ctx.Token,
		http:   &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}, nil
}

// APIError carries the server's structured error response.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
}

// IsUnauthorized reports whether err is a 401 from the server.
func IsUnauthorized(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusUnauthorized
	}
	return false
}

func (c *Client) do(method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.server+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	// Read maxResponseBytes+1 so that hitting the cap surfaces as an
	// explicit "too large" error instead of a confusing JSON parse failure
	// on a silently-truncated body.
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if readErr != nil {
		return fmt.Errorf("read response body: %w", readErr)
	}
	if int64(len(respBody)) > maxResponseBytes {
		return fmt.Errorf("response from %s exceeds %d bytes", path, maxResponseBytes)
	}
	if resp.StatusCode >= 400 {
		return &APIError{Status: resp.StatusCode, Message: extractErrorMsg(respBody)}
	}
	if out != nil && len(respBody) > 0 && resp.StatusCode != http.StatusNoContent {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w (body: %s)", err, truncate(respBody, 200))
		}
	}
	return nil
}

func extractErrorMsg(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err == nil {
		// Try the common error-envelope conventions in order. Maxx itself
		// uses "error" but other reverse-proxies/middlewares in front may
		// use "message" or "detail" — surface whichever shows up first.
		for _, key := range []string{"error", "message", "detail"} {
			if v, ok := m[key].(string); ok && v != "" {
				return v
			}
		}
	}
	return strings.TrimSpace(string(body))
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

// ============ Auth ============

// LoginResponse mirrors the JSON returned by POST /api/admin/auth/login.
type LoginResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token"`
	User    struct {
		ID         uint64 `json:"id"`
		Username   string `json:"username"`
		TenantID   uint64 `json:"tenantID"`
		TenantName string `json:"tenantName"`
		Role       string `json:"role"`
	} `json:"user"`
}

// Login exchanges username+password for a JWT.
func (c *Client) Login(username, password string) (*LoginResponse, error) {
	body := map[string]string{"username": username, "password": password}
	var out LoginResponse
	if err := c.do(http.MethodPost, "/api/admin/auth/login", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// JWTExpiry parses the `exp` claim of a JWT without verifying the signature.
// Returns the zero time if it cannot be parsed.
func JWTExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some libs pad — try standard base64 too.
		payload, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

// ============ Providers ============

func (c *Client) ListProviders() ([]*domain.Provider, error) {
	var out []*domain.Provider
	return out, c.do(http.MethodGet, "/api/admin/providers", nil, &out)
}

func (c *Client) GetProvider(id uint64) (*domain.Provider, error) {
	var out domain.Provider
	return &out, c.do(http.MethodGet, fmt.Sprintf("/api/admin/providers/%d", id), nil, &out)
}

func (c *Client) CreateProvider(p *domain.Provider) (*domain.Provider, error) {
	var out domain.Provider
	return &out, c.do(http.MethodPost, "/api/admin/providers", p, &out)
}

func (c *Client) UpdateProvider(id uint64, p *domain.Provider) (*domain.Provider, error) {
	var out domain.Provider
	return &out, c.do(http.MethodPut, fmt.Sprintf("/api/admin/providers/%d", id), p, &out)
}

func (c *Client) DeleteProvider(id uint64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/admin/providers/%d", id), nil, nil)
}

func (c *Client) ExportProviders() ([]*domain.Provider, error) {
	var out []*domain.Provider
	return out, c.do(http.MethodGet, "/api/admin/providers/export", nil, &out)
}

// ImportProvidersResult is the loosely-typed response from the import endpoint.
type ImportProvidersResult map[string]any

func (c *Client) ImportProviders(providers []*domain.Provider) (ImportProvidersResult, error) {
	var out ImportProvidersResult
	return out, c.do(http.MethodPost, "/api/admin/providers/import", providers, &out)
}

// ============ API Tokens ============

func (c *Client) ListAPITokens() ([]*domain.APIToken, error) {
	var out []*domain.APIToken
	return out, c.do(http.MethodGet, "/api/admin/api-tokens", nil, &out)
}

func (c *Client) GetAPIToken(id uint64) (*domain.APIToken, error) {
	var out domain.APIToken
	return &out, c.do(http.MethodGet, fmt.Sprintf("/api/admin/api-tokens/%d", id), nil, &out)
}

// CreateAPITokenRequest is the create-token body.
type CreateAPITokenRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	ProjectID   uint64  `json:"projectID,omitempty"`
	ExpiresAt   *string `json:"expiresAt,omitempty"`
}

func (c *Client) CreateAPIToken(req CreateAPITokenRequest) (*domain.APITokenCreateResult, error) {
	var out domain.APITokenCreateResult
	return &out, c.do(http.MethodPost, "/api/admin/api-tokens", req, &out)
}

// UpdateAPITokenRequest mirrors the partial-update body accepted by the server.
type UpdateAPITokenRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	ProjectID   *uint64 `json:"projectID,omitempty"`
	IsEnabled   *bool   `json:"isEnabled,omitempty"`
	DevMode     *bool   `json:"devMode,omitempty"`
	ExpiresAt   *string `json:"expiresAt,omitempty"`
}

func (c *Client) UpdateAPIToken(id uint64, req UpdateAPITokenRequest) (*domain.APIToken, error) {
	var out domain.APIToken
	return &out, c.do(http.MethodPut, fmt.Sprintf("/api/admin/api-tokens/%d", id), req, &out)
}

func (c *Client) DeleteAPIToken(id uint64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/admin/api-tokens/%d", id), nil, nil)
}

// ============ Routes ============

func (c *Client) ListRoutes() ([]*domain.Route, error) {
	var out []*domain.Route
	return out, c.do(http.MethodGet, "/api/admin/routes", nil, &out)
}

func (c *Client) GetRoute(id uint64) (*domain.Route, error) {
	var out domain.Route
	return &out, c.do(http.MethodGet, fmt.Sprintf("/api/admin/routes/%d", id), nil, &out)
}

func (c *Client) CreateRoute(r *domain.Route) (*domain.Route, error) {
	var out domain.Route
	return &out, c.do(http.MethodPost, "/api/admin/routes", r, &out)
}

// UpdateRoute sends a partial JSON patch. The server picks up isEnabled,
// isNative, projectID, clientType, providerID, position, weight, retryConfigID.
func (c *Client) UpdateRoute(id uint64, patch map[string]any) (*domain.Route, error) {
	var out domain.Route
	return &out, c.do(http.MethodPut, fmt.Sprintf("/api/admin/routes/%d", id), patch, &out)
}

func (c *Client) DeleteRoute(id uint64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/admin/routes/%d", id), nil, nil)
}

// ============ Routing Strategies ============

func (c *Client) ListRoutingStrategies() ([]*domain.RoutingStrategy, error) {
	var out []*domain.RoutingStrategy
	return out, c.do(http.MethodGet, "/api/admin/routing-strategies", nil, &out)
}

func (c *Client) GetRoutingStrategy(id uint64) (*domain.RoutingStrategy, error) {
	var out domain.RoutingStrategy
	return &out, c.do(http.MethodGet, fmt.Sprintf("/api/admin/routing-strategies/%d", id), nil, &out)
}

func (c *Client) CreateRoutingStrategy(s *domain.RoutingStrategy) (*domain.RoutingStrategy, error) {
	var out domain.RoutingStrategy
	return &out, c.do(http.MethodPost, "/api/admin/routing-strategies", s, &out)
}

func (c *Client) UpdateRoutingStrategy(id uint64, s *domain.RoutingStrategy) (*domain.RoutingStrategy, error) {
	var out domain.RoutingStrategy
	return &out, c.do(http.MethodPut, fmt.Sprintf("/api/admin/routing-strategies/%d", id), s, &out)
}

func (c *Client) DeleteRoutingStrategy(id uint64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/admin/routing-strategies/%d", id), nil, nil)
}

// ============ Users ============

func (c *Client) ListUsers() ([]*domain.User, error) {
	var out []*domain.User
	return out, c.do(http.MethodGet, "/api/admin/users", nil, &out)
}

func (c *Client) GetUser(id uint64) (*domain.User, error) {
	var out domain.User
	return &out, c.do(http.MethodGet, fmt.Sprintf("/api/admin/users/%d", id), nil, &out)
}

// CreateUserRequest body for POST /api/admin/users.
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

func (c *Client) CreateUser(req CreateUserRequest) (*domain.User, error) {
	var out domain.User
	return &out, c.do(http.MethodPost, "/api/admin/users", req, &out)
}

// UpdateUserRequest body for PUT /api/admin/users/{id}. nil pointer means
// "leave field alone"; the caller is responsible for only setting fields the
// user actually asked to change. The server treats empty/missing string
// fields as "no change" (handleUpdateUser uses `if body.Username != ""`).
type UpdateUserRequest struct {
	Username *string `json:"username,omitempty"`
	Role     *string `json:"role,omitempty"`
	Status   *string `json:"status,omitempty"`
}

func (c *Client) UpdateUser(id uint64, req UpdateUserRequest) (*domain.User, error) {
	var out domain.User
	return &out, c.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d", id), req, &out)
}

func (c *Client) UpdateUserPassword(id uint64, newPassword string) error {
	return c.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d/password", id),
		map[string]string{"password": newPassword}, nil)
}

func (c *Client) ApproveUser(id uint64) (*domain.User, error) {
	var out domain.User
	return &out, c.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d/approve", id), nil, &out)
}

func (c *Client) DeleteUser(id uint64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/admin/users/%d", id), nil, nil)
}

// ============ Invite Codes ============

func (c *Client) ListInviteCodes() ([]*domain.InviteCode, error) {
	var out []*domain.InviteCode
	return out, c.do(http.MethodGet, "/api/admin/invite-codes", nil, &out)
}

func (c *Client) GetInviteCode(id uint64) (*domain.InviteCode, error) {
	var out domain.InviteCode
	return &out, c.do(http.MethodGet, fmt.Sprintf("/api/admin/invite-codes/%d", id), nil, &out)
}

// CreateInviteCodesRequest body for POST /api/admin/invite-codes.
type CreateInviteCodesRequest struct {
	Count     int     `json:"count"`
	MaxUses   *uint64 `json:"maxUses,omitempty"`
	ExpiresAt *string `json:"expiresAt,omitempty"`
	Note      string  `json:"note,omitempty"`
}

func (c *Client) CreateInviteCodes(req CreateInviteCodesRequest) (*domain.InviteCodeCreateResult, error) {
	var out domain.InviteCodeCreateResult
	return &out, c.do(http.MethodPost, "/api/admin/invite-codes", req, &out)
}

// UpdateInviteCodeRequest body for PUT /api/admin/invite-codes/{id}.
type UpdateInviteCodeRequest struct {
	Status    *string `json:"status,omitempty"`
	MaxUses   *uint64 `json:"maxUses,omitempty"`
	ExpiresAt *string `json:"expiresAt,omitempty"`
	Note      *string `json:"note,omitempty"`
}

func (c *Client) UpdateInviteCode(id uint64, req UpdateInviteCodeRequest) (*domain.InviteCode, error) {
	var out domain.InviteCode
	return &out, c.do(http.MethodPut, fmt.Sprintf("/api/admin/invite-codes/%d", id), req, &out)
}

func (c *Client) DeleteInviteCode(id uint64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/admin/invite-codes/%d", id), nil, nil)
}

func (c *Client) ListInviteCodeUsages(id uint64) ([]*domain.InviteCodeUsage, error) {
	var out []*domain.InviteCodeUsage
	return out, c.do(http.MethodGet, fmt.Sprintf("/api/admin/invite-codes/%d/usages", id), nil, &out)
}

// ============ Settings ============

// ListSettings returns the server's flat key/value map of system settings.
func (c *Client) ListSettings() (map[string]string, error) {
	out := map[string]string{}
	return out, c.do(http.MethodGet, "/api/admin/settings", nil, &out)
}

// Setting is the flat shape returned by GET /api/admin/settings/{key} and
// echoed by writes.
type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (c *Client) GetSetting(key string) (*Setting, error) {
	var out Setting
	return &out, c.do(http.MethodGet, "/api/admin/settings/"+url.PathEscape(key), nil, &out)
}

func (c *Client) SetSetting(key, value string) (*Setting, error) {
	var out Setting
	return &out, c.do(http.MethodPut, "/api/admin/settings/"+url.PathEscape(key),
		map[string]string{"value": value}, &out)
}

func (c *Client) DeleteSetting(key string) error {
	return c.do(http.MethodDelete, "/api/admin/settings/"+url.PathEscape(key), nil, nil)
}
