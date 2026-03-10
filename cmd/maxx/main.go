package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/client"
	_ "github.com/awsl-project/maxx/internal/adapter/provider/claude" // Register claude adapter
	_ "github.com/awsl-project/maxx/internal/adapter/provider/custom" // Register custom adapter
	_ "github.com/awsl-project/maxx/internal/adapter/provider/kiro"   // Register kiro adapter
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/core"
	"github.com/awsl-project/maxx/internal/executor"
	"github.com/awsl-project/maxx/internal/handler"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/awsl-project/maxx/internal/service"
	"github.com/awsl-project/maxx/internal/stats"
	"github.com/awsl-project/maxx/internal/version"
	"github.com/awsl-project/maxx/internal/waiter"
)

// getDefaultDataDir returns the default data directory path (~/.config/maxx)
func getDefaultDataDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home dir is unavailable
		return "."
	}
	return filepath.Join(homeDir, ".config", "maxx")
}

// generateInstanceID generates a unique instance ID for this server run
func generateInstanceID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
}

func main() {
	// Parse flags
	addr := flag.String("addr", ":9880", "Server address")
	dataDir := flag.String("data", "", "Data directory for database and logs (default: ~/.config/maxx)")
	showVersion := flag.Bool("version", false, "Show version information and exit")
	flag.Parse()

	// Show version and exit if requested
	if *showVersion {
		fmt.Println("maxx", version.Full())
		os.Exit(0)
	}

	// Determine data directory: CLI flag > env var > default
	var dataDirPath string
	if *dataDir != "" {
		dataDirPath = *dataDir
	} else if envDataDir := os.Getenv("MAXX_DATA_DIR"); envDataDir != "" {
		dataDirPath = envDataDir
	} else {
		dataDirPath = getDefaultDataDir()
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDirPath, 0755); err != nil {
		log.Fatalf("Failed to create data directory %s: %v", dataDirPath, err)
	}

	// Construct database and log paths
	dbPath := filepath.Join(dataDirPath, "maxx.db")
	logPath := filepath.Join(dataDirPath, "maxx.log")

	// Initialize database (DSN > default SQLite path)
	var db *sqlite.DB
	var err error
	if dsn := os.Getenv("MAXX_DSN"); dsn != "" {
		log.Printf("Using database DSN from MAXX_DSN environment variable")
		db, err = sqlite.NewDBWithDSN(dsn)
	} else {
		db, err = sqlite.NewDB(dbPath)
	}
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
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
	antigravityQuotaRepo := sqlite.NewAntigravityQuotaRepository(db)
	codexQuotaRepo := sqlite.NewCodexQuotaRepository(db)
	cooldownRepo := sqlite.NewCooldownRepository(db)
	failureCountRepo := sqlite.NewFailureCountRepository(db)
	apiTokenRepo := sqlite.NewAPITokenRepository(db)
	modelMappingRepo := sqlite.NewModelMappingRepository(db)
	usageStatsRepo := sqlite.NewUsageStatsRepository(db)
	responseModelRepo := sqlite.NewResponseModelRepository(db)
	modelPriceRepo := sqlite.NewModelPriceRepository(db)
	tenantRepo := sqlite.NewTenantRepository(db)
	userRepo := sqlite.NewUserRepository(db)
	inviteCodeRepo := sqlite.NewInviteCodeRepository(db)
	inviteCodeUsageRepo := sqlite.NewInviteCodeUsageRepository(db)

	// Initialize cooldown manager with database persistence
	cooldown.Default().SetRepository(cooldownRepo)
	cooldown.Default().SetFailureCountRepository(failureCountRepo)
	if err := cooldown.Default().LoadFromDatabase(); err != nil {
		log.Printf("Warning: Failed to load cooldowns from database: %v", err)
	}

	// Generate instance ID and mark stale requests as failed
	instanceID := generateInstanceID()
	startupStep := time.Now()
	log.Printf("[Startup] Marking stale requests as failed...")
	if count, err := proxyRequestRepo.MarkStaleAsFailed(instanceID); err != nil {
		log.Printf("Warning: Failed to mark stale requests: %v", err)
	} else {
		log.Printf("[Startup] Marked %d stale requests as failed (%v)", count, time.Since(startupStep))
	}
	// Also mark stale upstream attempts as failed
	startupStep = time.Now()
	log.Printf("[Startup] Marking stale upstream attempts as failed...")
	if count, err := attemptRepo.MarkStaleAttemptsFailed(); err != nil {
		log.Printf("Warning: Failed to mark stale attempts: %v", err)
	} else {
		log.Printf("[Startup] Marked %d stale upstream attempts as failed (%v)", count, time.Since(startupStep))
	}
	// Fix legacy failed requests/attempts without end_time
	startupStep = time.Now()
	log.Printf("[Startup] Fixing failed requests without end_time...")
	if count, err := proxyRequestRepo.FixFailedRequestsWithoutEndTime(); err != nil {
		log.Printf("Warning: Failed to fix failed requests without end_time: %v", err)
	} else {
		log.Printf("[Startup] Fixed %d failed requests without end_time (%v)", count, time.Since(startupStep))
	}
	startupStep = time.Now()
	log.Printf("[Startup] Fixing failed attempts without end_time...")
	if count, err := attemptRepo.FixFailedAttemptsWithoutEndTime(); err != nil {
		log.Printf("Warning: Failed to fix failed attempts without end_time: %v", err)
	} else {
		log.Printf("[Startup] Fixed %d failed attempts without end_time (%v)", count, time.Since(startupStep))
	}

	// Create cached repositories
	cachedProviderRepo := cached.NewProviderRepository(providerRepo)
	cachedRouteRepo := cached.NewRouteRepository(routeRepo)
	cachedRetryConfigRepo := cached.NewRetryConfigRepository(retryConfigRepo)
	cachedRoutingStrategyRepo := cached.NewRoutingStrategyRepository(routingStrategyRepo)
	cachedSessionRepo := cached.NewSessionRepository(sessionRepo)
	cachedProjectRepo := cached.NewProjectRepository(projectRepo)
	cachedAPITokenRepo := cached.NewAPITokenRepository(apiTokenRepo)
	cachedModelMappingRepo := cached.NewModelMappingRepository(modelMappingRepo)

	// Load cached data
	startupStep = time.Now()
	log.Printf("[Startup] Loading caches...")
	if err := cachedProviderRepo.Load(); err != nil {
		log.Printf("Warning: Failed to load providers cache: %v", err)
	}
	if err := cachedRouteRepo.Load(); err != nil {
		log.Printf("Warning: Failed to load routes cache: %v", err)
	}
	if err := cachedRetryConfigRepo.Load(); err != nil {
		log.Printf("Warning: Failed to load retry configs cache: %v", err)
	}
	if err := cachedRoutingStrategyRepo.Load(); err != nil {
		log.Printf("Warning: Failed to load routing strategies cache: %v", err)
	}
	if err := cachedProjectRepo.Load(); err != nil {
		log.Printf("Warning: Failed to load projects cache: %v", err)
	}
	if err := cachedAPITokenRepo.Load(); err != nil {
		log.Printf("Warning: Failed to load API tokens cache: %v", err)
	}
	if err := cachedModelMappingRepo.Load(); err != nil {
		log.Printf("Warning: Failed to load model mappings cache: %v", err)
	}
	log.Printf("[Startup] Caches loaded (%v)", time.Since(startupStep))

	// Create router
	r := router.NewRouter(cachedRouteRepo, cachedProviderRepo, cachedRoutingStrategyRepo, cachedRetryConfigRepo, cachedProjectRepo)

	// Initialize provider adapters
	startupStep = time.Now()
	log.Printf("[Startup] Initializing provider adapters...")
	if err := r.InitAdapters(); err != nil {
		log.Printf("Warning: Failed to initialize adapters: %v", err)
	}
	log.Printf("[Startup] Provider adapters initialized (%v)", time.Since(startupStep))

	// Start cooldown cleanup goroutine with graceful shutdown support
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-cleanupCtx.Done():
				log.Println("[Cooldown] Background cleanup stopped")
				return
			case <-ticker.C:
				before := len(cooldown.Default().GetAllCooldowns())
				cooldown.Default().CleanupExpired()
				after := len(cooldown.Default().GetAllCooldowns())

				if before != after {
					log.Printf("[Cooldown] Cleanup completed: removed %d expired entries", before-after)
				}
			}
		}
	}()
	log.Println("[Cooldown] Background cleanup started (runs every 1 hour)")

	// Create WebSocket hub
	wsHub := handler.NewWebSocketHub()

	// Create Antigravity task service for periodic quota refresh and auto-sorting
	antigravityTaskSvc := service.NewAntigravityTaskService(
		cachedProviderRepo,
		cachedRouteRepo,
		antigravityQuotaRepo,
		settingRepo,
		proxyRequestRepo,
		tenantRepo,
		wsHub,
	)

	// Create Codex task service for periodic quota refresh and auto-sorting
	codexTaskSvc := service.NewCodexTaskService(
		cachedProviderRepo,
		cachedRouteRepo,
		codexQuotaRepo,
		settingRepo,
		proxyRequestRepo,
		tenantRepo,
		wsHub,
	)

	// Start background tasks
	core.StartBackgroundTasks(core.BackgroundTaskDeps{
		DB:                 db,
		UsageStats:         usageStatsRepo,
		ProxyRequest:       proxyRequestRepo,
		AttemptRepo:        attemptRepo,
		Settings:           settingRepo,
		AntigravityTaskSvc: antigravityTaskSvc,
		CodexTaskSvc:       codexTaskSvc,
	})

	// Ensure default tenant exists
	if _, err := tenantRepo.GetDefault(); err != nil {
		log.Fatalf("Failed to verify default tenant (migration v3 may be missing): %v", err)
	}

	// Setup log output to broadcast via WebSocket
	logWriter := handler.NewWebSocketLogWriter(wsHub, os.Stdout, logPath)
	log.SetOutput(logWriter)

	// Create project waiter for force project binding
	projectWaiter := waiter.NewProjectWaiter(cachedSessionRepo, settingRepo, wsHub)

	// Create stats aggregator
	statsAggregator := stats.NewStatsAggregator(usageStatsRepo)

	// Create executor
	requestExecutor := executor.NewExecutor(r, proxyRequestRepo, attemptRepo, cachedRetryConfigRepo, cachedSessionRepo, cachedModelMappingRepo, settingRepo, wsHub, projectWaiter, instanceID, statsAggregator)

	// Create client adapter
	clientAdapter := client.NewAdapter()

	// Create admin service
	pprofMgr := core.NewPprofManager(settingRepo)
	adminService := service.NewAdminService(
		cachedProviderRepo,
		cachedRouteRepo,
		cachedProjectRepo, // Use cached repository so updates are visible to Router
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
		*addr,
		r, // Router implements ProviderAdapterRefresher interface
		wsHub,
		pprofMgr, // Pprof reloader
	)

	// Start pprof manager (will check system settings)
	if err := pprofMgr.Start(context.Background()); err != nil {
		log.Printf("Warning: Failed to start pprof manager: %v", err)
	}

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
		r, // Router implements ProviderAdapterRefresher interface
	)

	// Determine if authentication is enabled based on MAXX_ADMIN_PASSWORD
	authEnabled := os.Getenv(handler.AdminPasswordEnvKey) != ""
	var authMiddleware *handler.AuthMiddleware

	if authEnabled {
		// Ensure an active admin user exists (panic on failure since all operations require auth)
		if err := core.SeedDefaultAdmin(userRepo); err != nil {
			log.Fatalf("Failed to seed default admin: %v", err)
		}
		authMiddleware = handler.NewAuthMiddleware(settingRepo)
		log.Println("Admin API authentication is enabled (multi-user mode)")
	} else {
		log.Println("Admin API authentication is disabled (no MAXX_ADMIN_PASSWORD set)")
	}

	// Create token auth middleware
	tokenAuthMiddleware := handler.NewTokenAuthMiddleware(cachedAPITokenRepo, settingRepo)
	if tokenAuthMiddleware.IsEnabled() {
		log.Println("Proxy token authentication is enabled")
	}

	// Create request tracker for graceful shutdown
	requestTracker := core.NewRequestTracker()

	// Create handlers
	proxyHandler := handler.NewProxyHandler(clientAdapter, requestExecutor, cachedSessionRepo, tokenAuthMiddleware)
	proxyHandler.SetRequestTracker(requestTracker)
	adminHandler := handler.NewAdminHandler(adminService, backupService, logPath)
	adminHandler.SetUserRepo(userRepo)
	adminHandler.SetAuthEnabled(authEnabled)
	authHandler := handler.NewAuthHandler(
		authMiddleware,
		userRepo,
		tenantRepo,
		inviteCodeRepo,
		inviteCodeUsageRepo,
		authEnabled,
	)
	antigravityHandler := handler.NewAntigravityHandler(adminService, antigravityQuotaRepo, wsHub)
	antigravityHandler.SetTaskService(antigravityTaskSvc)
	kiroHandler := handler.NewKiroHandler(adminService)
	codexHandler := handler.NewCodexHandler(adminService, codexQuotaRepo, wsHub)
	codexHandler.SetTaskService(codexTaskSvc)
	claudeHandler := handler.NewClaudeHandler(adminService, wsHub)

	// Use already-created cached project repository for project proxy handler
	modelsHandler := handler.NewModelsHandler(responseModelRepo, cachedProviderRepo, cachedModelMappingRepo)
	projectProxyHandler := handler.NewProjectProxyHandler(proxyHandler, modelsHandler, cachedProjectRepo)

	// Setup routes
	mux := http.NewServeMux()

	// Admin auth endpoint (no authentication required for this endpoint)
	mux.Handle("/api/admin/auth/", http.StripPrefix("/api", authHandler))

	// Admin API routes with authentication middleware
	if authMiddleware != nil {
		mux.Handle("/api/admin/", http.StripPrefix("/api", authMiddleware.Wrap(adminHandler)))
	} else {
		mux.Handle("/api/admin/", http.StripPrefix("/api", handler.NoAuthMiddleware(adminHandler)))
	}

	// Other API routes (no authentication required)
	mux.Handle("/api/antigravity/", http.StripPrefix("/api", antigravityHandler))
	mux.Handle("/api/kiro/", http.StripPrefix("/api", kiroHandler))
	mux.Handle("/api/codex/", http.StripPrefix("/api", codexHandler))
	mux.Handle("/api/claude/", http.StripPrefix("/api", claudeHandler))

	// Proxy routes - catch all AI API endpoints
	// Claude API
	mux.Handle("/v1/messages", proxyHandler)
	mux.Handle("/v1/messages/", proxyHandler)
	// OpenAI API
	mux.Handle("/v1/chat/completions", proxyHandler)
	// Codex API
	mux.Handle("/responses", proxyHandler)
	mux.Handle("/responses/", proxyHandler)
	mux.Handle("/v1/responses", proxyHandler)
	mux.Handle("/v1/responses/", proxyHandler)
	// Gemini API (Google AI Studio style)
	mux.Handle("/v1beta/models/", proxyHandler)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// WebSocket endpoint
	mux.HandleFunc("/ws", wsHub.HandleWebSocket)

	// Serve static files (Web UI) with project proxy support - must be last (default route)
	staticHandler := handler.NewStaticHandler()
	combinedHandler := handler.NewCombinedHandler(projectProxyHandler, staticHandler)
	mux.Handle("/", combinedHandler)

	// Wrap with logging middleware
	loggedMux := handler.LoggingMiddleware(mux)

	// Create HTTP server
	server := &http.Server{
		Addr:    *addr,
		Handler: loggedMux,
	}

	// Initialize OAuth callback servers (start on-demand)
	codexOAuthServer := core.NewCodexOAuthServer(codexHandler)
	codexHandler.SetOAuthServer(codexOAuthServer)
	claudeOAuthServer := core.NewClaudeOAuthServer(claudeHandler)
	claudeHandler.SetOAuthServer(claudeOAuthServer)

	var restartInProgress int32

	shutdownServer := func(reason string) {
		log.Printf("Initiating graceful shutdown (%s)...", reason)

		// Step 1: Wait for active proxy requests to complete
		activeCount := requestTracker.ActiveCount()
		if activeCount > 0 {
			log.Printf("Waiting for %d active proxy requests to complete...", activeCount)
			completed := requestTracker.GracefulShutdown(core.GracefulShutdownTimeout)
			if !completed {
				log.Printf("Graceful shutdown timeout, some requests may be interrupted")
			} else {
				log.Printf("All proxy requests completed successfully")
			}
		} else {
			// Mark as shutting down to reject new requests
			requestTracker.GracefulShutdown(0)
			log.Printf("No active proxy requests")
		}

		// Step 2: Stop pprof manager
		shutdownCtx, cancel := context.WithTimeout(context.Background(), core.HTTPShutdownTimeout)
		defer cancel()

		// Stop background cleanup task
		cleanupCancel()

		// Stop pprof manager
		if err := pprofMgr.Stop(shutdownCtx); err != nil {
			log.Printf("Warning: Failed to stop pprof manager: %v", err)
		}

		// Stop OAuth servers
		if err := codexOAuthServer.Stop(shutdownCtx); err != nil {
			log.Printf("Warning: Failed to stop Codex OAuth server: %v", err)
		}
		if err := claudeOAuthServer.Stop(shutdownCtx); err != nil {
			log.Printf("Warning: Failed to stop Claude OAuth server: %v", err)
		}

		// Step 3: Shutdown HTTP server
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server graceful shutdown failed: %v, forcing close", err)
			if closeErr := server.Close(); closeErr != nil {
				log.Printf("Force close error: %v", closeErr)
			}
		}
	}

	restartServer := func() error {
		if !atomic.CompareAndSwapInt32(&restartInProgress, 0, 1) {
			return fmt.Errorf("restart already in progress")
		}

		shutdownServer("restart")

		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to locate executable: %w", err)
		}

		cmd := exec.Command(executable, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start new process: %w", err)
		}

		log.Printf("[Admin] Started new process (pid=%d). Exiting current process.", cmd.Process.Pid)
		os.Exit(0)
		return nil
	}

	adminHandler.SetRestartFunc(restartServer)

	// Start server in goroutine
	log.Printf("Starting Maxx server %s on %s", version.Info(), *addr)
	log.Printf("Data directory: %s", dataDirPath)
	log.Printf("  Database: %s", dbPath)
	log.Printf("  Log file: %s", logPath)
	log.Printf("Admin API: http://localhost%s/api/admin/", *addr)
	log.Printf("WebSocket: ws://localhost%s/ws", *addr)
	log.Printf("Proxy endpoints:")
	log.Printf("  Claude: http://localhost%s/v1/messages", *addr)
	log.Printf("  OpenAI: http://localhost%s/v1/chat/completions", *addr)
	log.Printf("  Codex:  http://localhost%s/v1/responses", *addr)
	log.Printf("  Gemini: http://localhost%s/v1beta/models/{model}:generateContent", *addr)
	log.Printf("Project proxy: http://localhost%s/project/{project-slug}/v1/messages (etc.)", *addr)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal (SIGINT or SIGTERM)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %v, initiating graceful shutdown...", sig)
	shutdownServer(fmt.Sprintf("signal %v", sig))

	log.Printf("Server stopped")
}
