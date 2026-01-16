package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/client"
	"github.com/awsl-project/maxx/internal/adapter/provider/antigravity"
	_ "github.com/awsl-project/maxx/internal/adapter/provider/custom" // Register custom adapter
	_ "github.com/awsl-project/maxx/internal/adapter/provider/kiro"   // Register kiro adapter
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/executor"
	"github.com/awsl-project/maxx/internal/handler"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/awsl-project/maxx/internal/service"
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

	// Initialize database
	db, err := sqlite.NewDB(dbPath)
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
	cooldownRepo := sqlite.NewCooldownRepository(db)
	failureCountRepo := sqlite.NewFailureCountRepository(db)

	// Initialize cooldown manager with database persistence
	cooldown.Default().SetRepository(cooldownRepo)
	cooldown.Default().SetFailureCountRepository(failureCountRepo)
	if err := cooldown.Default().LoadFromDatabase(); err != nil {
		log.Printf("Warning: Failed to load cooldowns from database: %v", err)
	}

	// Generate instance ID and mark stale requests as failed
	instanceID := generateInstanceID()
	if count, err := proxyRequestRepo.MarkStaleAsFailed(instanceID); err != nil {
		log.Printf("Warning: Failed to mark stale requests: %v", err)
	} else if count > 0 {
		log.Printf("Marked %d stale requests as failed", count)
	}

	// Create cached repositories
	cachedProviderRepo := cached.NewProviderRepository(providerRepo)
	cachedRouteRepo := cached.NewRouteRepository(routeRepo)
	cachedRetryConfigRepo := cached.NewRetryConfigRepository(retryConfigRepo)
	cachedRoutingStrategyRepo := cached.NewRoutingStrategyRepository(routingStrategyRepo)
	cachedSessionRepo := cached.NewSessionRepository(sessionRepo)
	cachedProjectRepo := cached.NewProjectRepository(projectRepo)

	// Load cached data
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

	// Create router
	r := router.NewRouter(cachedRouteRepo, cachedProviderRepo, cachedRoutingStrategyRepo, cachedRetryConfigRepo, cachedProjectRepo)

	// Initialize provider adapters
	if err := r.InitAdapters(); err != nil {
		log.Printf("Warning: Failed to initialize adapters: %v", err)
	}

	// Start cooldown cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			before := len(cooldown.Default().GetAllCooldowns())
			cooldown.Default().CleanupExpired()
			after := len(cooldown.Default().GetAllCooldowns())

			if before != after {
				log.Printf("[Cooldown] Cleanup completed: removed %d expired entries", before-after)
			}
		}
	}()
	log.Println("[Cooldown] Background cleanup started (runs every 1 hour)")

	// Create WebSocket hub
	wsHub := handler.NewWebSocketHub()

	// Setup log output to broadcast via WebSocket
	logWriter := handler.NewWebSocketLogWriter(wsHub, os.Stdout, logPath)
	log.SetOutput(logWriter)

	// Create project waiter for force project binding
	projectWaiter := waiter.NewProjectWaiter(cachedSessionRepo, settingRepo, wsHub)

	// Create executor
	exec := executor.NewExecutor(r, proxyRequestRepo, attemptRepo, cachedRetryConfigRepo, cachedSessionRepo, wsHub, projectWaiter, instanceID)

	// Create client adapter
	clientAdapter := client.NewAdapter()

	// Create admin service
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
		*addr,
		r, // Router implements ProviderAdapterRefresher interface
	)

	// Initialize Antigravity global settings getter
	antigravity.SetGlobalSettingsGetter(func() (*antigravity.GlobalSettings, error) {
		rulesJSON, _ := settingRepo.Get(domain.SettingKeyAntigravityModelMapping)
		rules, err := antigravity.ParseModelMappingRules(rulesJSON)
		if err != nil {
			return nil, err
		}
		return &antigravity.GlobalSettings{
			ModelMappingRules: rules,
		}, nil
	})

	// Create auth middleware
	authMiddleware := handler.NewAuthMiddleware()
	if authMiddleware.IsEnabled() {
		log.Println("Admin API authentication is enabled")
	} else {
		log.Println("Admin API authentication is disabled (set MAXX_ADMIN_PASSWORD to enable)")
	}

	// Create handlers
	proxyHandler := handler.NewProxyHandler(clientAdapter, exec, cachedSessionRepo)
	adminHandler := handler.NewAdminHandler(adminService, logPath)
	authHandler := handler.NewAuthHandler(authMiddleware)
	antigravityHandler := handler.NewAntigravityHandler(adminService, antigravityQuotaRepo, wsHub)
	kiroHandler := handler.NewKiroHandler(adminService)

	// Use already-created cached project repository for project proxy handler
	projectProxyHandler := handler.NewProjectProxyHandler(proxyHandler, cachedProjectRepo)

	// Setup routes
	mux := http.NewServeMux()

	// Admin auth endpoint (no authentication required for this endpoint)
	mux.Handle("/api/admin/auth/", http.StripPrefix("/api", authHandler))

	// Admin API routes with authentication middleware
	mux.Handle("/api/admin/", http.StripPrefix("/api", authMiddleware.Wrap(adminHandler)))

	// Other API routes (no authentication required)
	mux.Handle("/api/antigravity/", http.StripPrefix("/api", antigravityHandler))
	mux.Handle("/api/kiro/", http.StripPrefix("/api", kiroHandler))

	// Proxy routes - catch all AI API endpoints
	// Claude API
	mux.Handle("/v1/messages", proxyHandler)
	// OpenAI API
	mux.Handle("/v1/chat/completions", proxyHandler)
	// Codex API
	mux.Handle("/responses", proxyHandler)
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

	// Start server
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
	log.Printf("Project proxy: http://localhost%s/{project-slug}/v1/messages (etc.)", *addr)

	if err := http.ListenAndServe(*addr, loggedMux); err != nil {
		log.Printf("Server error: %v", err)
		os.Exit(1)
	}
}
