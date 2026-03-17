package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	client "github.com/awsl-project/maxx/internal/adapter/client"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/executor"
	"github.com/awsl-project/maxx/internal/handler"
	"github.com/awsl-project/maxx/internal/health"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/awsl-project/maxx/internal/service"
	"github.com/awsl-project/maxx/internal/stats"
	"github.com/awsl-project/maxx/internal/waiter"
	"golang.org/x/crypto/bcrypt"

	"github.com/awsl-project/maxx/internal/domain"

	// Register adapter factories via init()
	_ "github.com/awsl-project/maxx/internal/adapter/provider/custom"

	// Register converters via init()
	_ "github.com/awsl-project/maxx/internal/converter"
)

// ProxyTestEnv extends TestEnv with a full proxy pipeline.
type ProxyTestEnv struct {
	t      *testing.T
	Server *httptest.Server
	DB     *sqlite.DB
	Token  string // Admin JWT token
}

type ProxyTestEnvOptions struct {
	AttemptBudget *executor.AttemptBudget
}

// NewProxyTestEnv creates a test environment that includes the proxy handler,
// suitable for end-to-end proxy integration testing.
func NewProxyTestEnv(t *testing.T) *ProxyTestEnv {
	return newProxyTestEnv(t, ProxyTestEnvOptions{})
}

func NewProxyTestEnvWithAttemptBudget(t *testing.T, budget executor.AttemptBudget) *ProxyTestEnv {
	return newProxyTestEnv(t, ProxyTestEnvOptions{
		AttemptBudget: &budget,
	})
}

func newProxyTestEnv(t *testing.T, opts ProxyTestEnvOptions) *ProxyTestEnv {
	t.Helper()

	// Clear global cooldown state from previous tests (singleton is shared across tests)
	for i := uint64(1); i <= 10; i++ {
		cooldown.Default().ClearCooldown(i, "")
	}

	dsn := fmt.Sprintf("file:proxytest_%d?mode=memory&cache=shared&_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)", time.Now().UnixNano())
	db, err := sqlite.NewDBWithDSN(dsn)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create repositories
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
	inviteCodeRepo := sqlite.NewInviteCodeRepository(db)
	inviteCodeUsageRepo := sqlite.NewInviteCodeUsageRepository(db)

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

	// Ensure default tenant
	if _, err := tenantRepo.GetDefault(); err != nil {
		t.Fatalf("Failed to verify default tenant: %v", err)
	}

	// Create admin user
	hash, err := bcrypt.GenerateFromPassword([]byte("test-admin-password"), bcrypt.MinCost)
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

	// Set JWT secret
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
	providerHealthTracker := health.NewTracker()

	// Create Router (for proxy pipeline)
	r := router.NewRouter(cachedRouteRepo, cachedProviderRepo, cachedRoutingStrategyRepo, cachedRetryConfigRepo, cachedProjectRepo, providerHealthTracker)

	// Create admin service with Router as adapter refresher
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
		inviteCodeRepo,
		inviteCodeUsageRepo,
		cachedModelMappingRepo,
		usageStatsRepo,
		responseModelRepo,
		modelPriceRepo,
		":9880",
		r, // Router as adapter refresher (not nil, so adapters refresh on provider create)
		wsHub,
		nil, // no pprof reloader
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
		r,
	)

	// Create project waiter and stats aggregator
	projectWaiter := waiter.NewProjectWaiter(cachedSessionRepo, settingRepo, wsHub)
	statsAggregator := stats.NewStatsAggregator(usageStatsRepo)

	// Create executor
	requestExecutor := executor.NewExecutor(r, proxyRequestRepo, attemptRepo, cachedRetryConfigRepo, cachedSessionRepo, cachedModelMappingRepo, settingRepo, wsHub, projectWaiter, "test-instance", statsAggregator, providerHealthTracker)
	if opts.AttemptBudget != nil {
		requestExecutor.SetAttemptBudget(*opts.AttemptBudget)
	}

	// Create client adapter
	clientAdapter := client.NewAdapter()

	// Create token auth middleware (disabled by default - no setting configured)
	tokenAuthMiddleware := handler.NewTokenAuthMiddleware(cachedAPITokenRepo, settingRepo)

	// Create proxy handler
	proxyHandler := handler.NewProxyHandler(clientAdapter, requestExecutor, cachedSessionRepo, tokenAuthMiddleware)

	// Create admin and auth handlers
	adminHandler := handler.NewAdminHandler(adminService, backupService, "")
	adminHandler.SetUserRepo(userRepo)
	authHandler := handler.NewAuthHandler(authMiddleware, userRepo, tenantRepo, inviteCodeRepo, inviteCodeUsageRepo, true)

	// Create models handler
	modelsHandler := handler.NewModelsHandler(responseModelRepo, cachedProviderRepo, cachedModelMappingRepo)

	// Setup routes (mirroring main.go)
	mux := http.NewServeMux()

	// Admin auth endpoint
	mux.Handle("/api/admin/auth/", http.StripPrefix("/api", authHandler))

	// Admin API routes with authentication
	mux.Handle("/api/admin/", http.StripPrefix("/api", authMiddleware.Wrap(adminHandler)))

	// Models endpoint
	mux.Handle("/v1/models", modelsHandler)

	// Proxy routes - all AI API endpoints
	mux.Handle("/v1/messages", proxyHandler)
	mux.Handle("/v1/messages/", proxyHandler)
	mux.Handle("/v1/chat/completions", proxyHandler)
	mux.Handle("/responses", proxyHandler)
	mux.Handle("/responses/", proxyHandler)
	mux.Handle("/v1/responses", proxyHandler)
	mux.Handle("/v1/responses/", proxyHandler)
	mux.Handle("/v1beta/models/", proxyHandler)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	server := httptest.NewServer(mux)

	env := &ProxyTestEnv{
		t:      t,
		Server: server,
		DB:     db,
		Token:  adminToken,
	}

	t.Cleanup(func() {
		server.Close()
		db.Close()
	})

	return env
}

// URL returns the full URL for a given path.
func (e *ProxyTestEnv) URL(path string) string {
	return e.Server.URL + path
}

// AdminPost sends an authenticated POST request with JSON body.
func (e *ProxyTestEnv) AdminPost(path string, body any) *http.Response {
	e.t.Helper()
	return e.doRequest(http.MethodPost, path, body, e.Token)
}

// AdminPut sends an authenticated PUT request with JSON body.
func (e *ProxyTestEnv) AdminPut(path string, body any) *http.Response {
	e.t.Helper()
	return e.doRequest(http.MethodPut, path, body, e.Token)
}

func (e *ProxyTestEnv) doRequest(method, path string, body any, token string) *http.Response {
	e.t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("Failed to marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	var req *http.Request
	var err error
	if bodyReader != nil {
		req, err = http.NewRequest(method, e.URL(path), bodyReader)
	} else {
		req, err = http.NewRequest(method, e.URL(path), nil)
	}
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

// ProxyPost sends a POST to a proxy endpoint with the given body and headers.
func (e *ProxyTestEnv) ProxyPost(path string, body any, headers map[string]string) *http.Response {
	e.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		e.t.Fatalf("Failed to marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, e.URL(path), bytes.NewReader(data))
	if err != nil {
		e.t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("Request failed: %v", err)
	}
	return resp
}
