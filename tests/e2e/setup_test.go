package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/handler"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/awsl-project/maxx/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// TestEnv encapsulates the full test environment for E2E tests.
type TestEnv struct {
	t      *testing.T
	Server *httptest.Server
	DB     *sqlite.DB
	Token  string // Admin JWT token
}

// NewTestEnv creates a fully assembled test environment mirroring cmd/maxx/main.go.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	// Use a unique file-based DSN per test for full isolation.
	// Each test gets its own database name so no shared state leaks between tests.
	dsn := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared&_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)", time.Now().UnixNano())
	db, err := sqlite.NewDBWithDSN(dsn)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create repositories (same order as main.go)
	providerRepo := sqlite.NewProviderRepository(db)
	routeRepo := sqlite.NewRouteRepository(db)
	projectRepo := sqlite.NewProjectRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	retryConfigRepo := sqlite.NewRetryConfigRepository(db)
	routingStrategyRepo := sqlite.NewRoutingStrategyRepository(db)
	proxyRequestRepo := sqlite.NewProxyRequestRepository(db)
	attemptRepo := sqlite.NewProxyUpstreamAttemptRepository(db)
	settingRepo := sqlite.NewSystemSettingRepository(db)
	apiTokenRepo := sqlite.NewAPITokenRepository(db)
	modelMappingRepo := sqlite.NewModelMappingRepository(db)
	usageStatsRepo := sqlite.NewUsageStatsRepository(db)
	responseModelRepo := sqlite.NewResponseModelRepository(db)
	modelPriceRepo := sqlite.NewModelPriceRepository(db)
	tenantRepo := sqlite.NewTenantRepository(db)
	userRepo := sqlite.NewUserRepository(db)

	// Create cached repositories
	cachedProviderRepo := cached.NewProviderRepository(providerRepo)
	cachedRouteRepo := cached.NewRouteRepository(routeRepo)
	cachedRetryConfigRepo := cached.NewRetryConfigRepository(retryConfigRepo)
	cachedRoutingStrategyRepo := cached.NewRoutingStrategyRepository(routingStrategyRepo)
	cachedProjectRepo := cached.NewProjectRepository(projectRepo)
	cachedSessionRepo := cached.NewSessionRepository(sessionRepo)
	cachedAPITokenRepo := cached.NewAPITokenRepository(apiTokenRepo)
	cachedModelMappingRepo := cached.NewModelMappingRepository(modelMappingRepo)

	// Load cached data
	_ = cachedProviderRepo.Load()
	_ = cachedRouteRepo.Load()
	_ = cachedRetryConfigRepo.Load()
	_ = cachedRoutingStrategyRepo.Load()
	_ = cachedProjectRepo.Load()
	_ = cachedAPITokenRepo.Load()
	_ = cachedModelMappingRepo.Load()

	// Ensure default tenant exists
	if _, err := tenantRepo.GetDefault(); err != nil {
		t.Fatalf("Failed to verify default tenant: %v", err)
	}

	// Create admin user with known password
	adminPassword := "test-admin-password"
	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	adminUser := &domain.User{
		TenantID:     domain.DefaultTenantID,
		Username:     "admin",
		PasswordHash: string(hash),
		Role:         domain.UserRoleAdmin,
		Status:       domain.UserStatusActive,
		IsDefault:    true,
	}
	if err := userRepo.Create(adminUser); err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}

	// Set JWT secret via system settings
	if err := settingRepo.Set(handler.SettingKeyJWTSecret, "e2e-test-jwt-secret"); err != nil {
		t.Fatalf("Failed to set JWT secret: %v", err)
	}

	// Create auth middleware and generate admin token
	authMiddleware := handler.NewAuthMiddleware(settingRepo)
	adminToken, err := authMiddleware.GenerateToken(adminUser)
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}

	// Create WebSocket hub
	wsHub := handler.NewWebSocketHub()

	// Create admin service
	adminService := service.NewAdminService(
		cachedProviderRepo,
		cachedRouteRepo,
		cachedProjectRepo,
		cachedSessionRepo,
		cachedRetryConfigRepo,
		cachedRoutingStrategyRepo,
		proxyRequestRepo,
		attemptRepo,
		settingRepo,
		cachedAPITokenRepo,
		cachedModelMappingRepo,
		usageStatsRepo,
		responseModelRepo,
		modelPriceRepo,
		":9880",
		nil, // no adapter refresher in tests
		wsHub,
		nil, // no pprof reloader in tests
	)

	// Create backup service
	backupService := service.NewBackupService(
		cachedProviderRepo,
		cachedRouteRepo,
		cachedProjectRepo,
		cachedRetryConfigRepo,
		cachedRoutingStrategyRepo,
		settingRepo,
		cachedAPITokenRepo,
		cachedModelMappingRepo,
		modelPriceRepo,
		nil, // no adapter refresher in tests
	)

	// Create handlers
	adminHandler := handler.NewAdminHandler(adminService, backupService, "")
	adminHandler.SetUserRepo(userRepo)
	authHandler := handler.NewAuthHandler(authMiddleware, userRepo, tenantRepo, true)

	// Create models handler
	modelsHandler := handler.NewModelsHandler(responseModelRepo, cachedProviderRepo, cachedModelMappingRepo)

	// Setup routes (mirroring main.go)
	mux := http.NewServeMux()

	// Admin auth endpoint (no authentication required)
	mux.Handle("/api/admin/auth/", http.StripPrefix("/api", authHandler))

	// Admin API routes with authentication middleware
	mux.Handle("/api/admin/", http.StripPrefix("/api", authMiddleware.Wrap(adminHandler)))

	// Models endpoint (public)
	mux.Handle("/v1/models", modelsHandler)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	server := httptest.NewServer(mux)

	env := &TestEnv{
		t:      t,
		Server: server,
		DB:     db,
		Token:  adminToken,
	}

	t.Cleanup(func() {
		env.Close()
	})

	return env
}

// Close cleans up the test environment.
func (e *TestEnv) Close() {
	e.Server.Close()
	e.DB.Close()
}

// URL returns the full URL for a given path.
func (e *TestEnv) URL(path string) string {
	return e.Server.URL + path
}

// AdminGet sends an authenticated GET request to the given path.
func (e *TestEnv) AdminGet(path string) *http.Response {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.URL(path), nil)
	if err != nil {
		e.t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// AdminPost sends an authenticated POST request with JSON body.
func (e *TestEnv) AdminPost(path string, body any) *http.Response {
	e.t.Helper()
	return e.adminRequest(http.MethodPost, path, body, e.Token)
}

// AdminPut sends an authenticated PUT request with JSON body.
func (e *TestEnv) AdminPut(path string, body any) *http.Response {
	e.t.Helper()
	return e.adminRequest(http.MethodPut, path, body, e.Token)
}

// AdminDelete sends an authenticated DELETE request.
func (e *TestEnv) AdminDelete(path string) *http.Response {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodDelete, e.URL(path), nil)
	if err != nil {
		e.t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// RequestWithToken sends a request with a specific token.
func (e *TestEnv) RequestWithToken(method, path string, body any, token string) *http.Response {
	e.t.Helper()
	return e.adminRequest(method, path, body, token)
}

// UnauthGet sends an unauthenticated GET request.
func (e *TestEnv) UnauthGet(path string) *http.Response {
	e.t.Helper()
	resp, err := http.Get(e.URL(path))
	if err != nil {
		e.t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// UnauthPost sends an unauthenticated POST request with JSON body.
func (e *TestEnv) UnauthPost(path string, body any) *http.Response {
	e.t.Helper()
	return e.adminRequest(http.MethodPost, path, body, "")
}

func (e *TestEnv) adminRequest(method, path string, body any, token string) *http.Response {
	e.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("Failed to marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, e.URL(path), bodyReader)
	if err != nil {
		e.t.Fatalf("Failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// DecodeJSON decodes a JSON response body into the given target.
func DecodeJSON(t *testing.T, resp *http.Response, target any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}
}

// ReadBody reads and returns the response body as a string.
func ReadBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	return string(data)
}

// AssertStatus asserts the HTTP response status code.
func AssertStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status %d, got %d, body: %s", expected, resp.StatusCode, string(body))
	}
}

// CreatePendingUser creates a user with pending status via the /apply endpoint and returns the username.
func (e *TestEnv) CreatePendingUser(username, password string) {
	e.t.Helper()
	resp := e.UnauthPost("/api/admin/auth/apply", map[string]string{
		"username": username,
		"password": password,
	})
	AssertStatus(e.t, resp, http.StatusCreated)
	resp.Body.Close()
}

// RawPost sends a POST request with a raw string body (for invalid JSON testing).
func (e *TestEnv) RawPost(path string, rawBody string) *http.Response {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.URL(path), bytes.NewBufferString(rawBody))
	if err != nil {
		e.t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// AdminRawPost sends an authenticated POST request with a raw string body (for invalid JSON testing).
func (e *TestEnv) AdminRawPost(path string, rawBody string) *http.Response {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.URL(path), bytes.NewBufferString(rawBody))
	if err != nil {
		e.t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// AdminRawPut sends an authenticated PUT request with a raw string body (for invalid JSON testing).
func (e *TestEnv) AdminRawPut(path string, rawBody string) *http.Response {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPut, e.URL(path), bytes.NewBufferString(rawBody))
	if err != nil {
		e.t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// GenerateExpiredToken generates an expired JWT token for testing.
func (e *TestEnv) GenerateExpiredToken() string {
	e.t.Helper()
	// Create a token that expired 1 hour ago using the same JWT secret
	claims := struct {
		jwt.RegisteredClaims
		UserID   uint64 `json:"uid"`
		TenantID uint64 `json:"tid"`
		Role     string `json:"role"`
	}{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			Issuer:    "maxx-admin",
		},
		UserID:   1,
		TenantID: 1,
		Role:     "admin",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("e2e-test-jwt-secret"))
	if err != nil {
		e.t.Fatalf("Failed to generate expired token: %v", err)
	}
	return signed
}

// UnauthPut sends an unauthenticated PUT request with JSON body.
func (e *TestEnv) UnauthPut(path string, body any) *http.Response {
	e.t.Helper()
	return e.adminRequest(http.MethodPut, path, body, "")
}
