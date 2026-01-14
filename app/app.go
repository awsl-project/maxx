package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/client"
	_ "github.com/awsl-project/maxx/internal/adapter/provider/antigravity"
	_ "github.com/awsl-project/maxx/internal/adapter/provider/custom"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/executor"
	"github.com/awsl-project/maxx/internal/handler"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/awsl-project/maxx/internal/service"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx            context.Context
	httpServer     *http.Server
	adminService   *service.AdminService
	antigravitySvc *AntigravityService
	wsHub          *handler.WebSocketHub
	db             *sqlite.DB
	dataDir        string
}

func NewApp() *App {
	return &App{}
}

var Instance = NewApp()

func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx

	log.Println("Starting Maxx desktop application...")

	err := a.initializeDataDir()
	if err != nil {
		log.Fatalf("Failed to initialize data directory: %v", err)
	}

	err = a.initializeDatabase()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	a.initializeServices()

	a.startBackgroundTasks()

	log.Println("Maxx application started successfully")
}

func (a *App) OnShutdown(ctx context.Context) {
	log.Println("Shutting down Maxx application...")

	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down HTTP server: %v", err)
		}
	}

	if a.db != nil {
		a.db.Close()
	}

	log.Println("Maxx application stopped")
}

func (a *App) initializeDataDir() error {
	var err error
	a.dataDir, err = os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get user config directory: %w", err)
	}

	a.dataDir = filepath.Join(a.dataDir, "maxx")

	if err := os.MkdirAll(a.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	log.Printf("Data directory: %s", a.dataDir)
	return nil
}

func (a *App) initializeDatabase() error {
	dbPath := filepath.Join(a.dataDir, "maxx.db")

	var err error
	a.db, err = sqlite.NewDB(dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	log.Printf("Database initialized: %s", dbPath)
	return nil
}

func (a *App) initializeServices() {
	providerRepo := sqlite.NewProviderRepository(a.db)
	routeRepo := sqlite.NewRouteRepository(a.db)
	projectRepo := sqlite.NewProjectRepository(a.db)
	sessionRepo := sqlite.NewSessionRepository(a.db)
	retryConfigRepo := sqlite.NewRetryConfigRepository(a.db)
	routingStrategyRepo := sqlite.NewRoutingStrategyRepository(a.db)
	proxyRequestRepo := sqlite.NewProxyRequestRepository(a.db)
	attemptRepo := sqlite.NewProxyUpstreamAttemptRepository(a.db)
	settingRepo := sqlite.NewSystemSettingRepository(a.db)
	antigravityQuotaRepo := sqlite.NewAntigravityQuotaRepository(a.db)
	cooldownRepo := sqlite.NewCooldownRepository(a.db)
	failureCountRepo := sqlite.NewFailureCountRepository(a.db)

	cooldown.Default().SetRepository(cooldownRepo)
	cooldown.Default().SetFailureCountRepository(failureCountRepo)
	if err := cooldown.Default().LoadFromDatabase(); err != nil {
		log.Printf("Warning: Failed to load cooldowns from database: %v", err)
	}

	cachedProviderRepo := cached.NewProviderRepository(providerRepo)
	cachedRouteRepo := cached.NewRouteRepository(routeRepo)
	cachedRetryConfigRepo := cached.NewRetryConfigRepository(retryConfigRepo)
	cachedRoutingStrategyRepo := cached.NewRoutingStrategyRepository(routingStrategyRepo)
	cachedSessionRepo := cached.NewSessionRepository(sessionRepo)
	cachedProjectRepo := cached.NewProjectRepository(projectRepo)

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

	r := router.NewRouter(cachedRouteRepo, cachedProviderRepo, cachedRoutingStrategyRepo, cachedRetryConfigRepo, cachedProjectRepo)

	if err := r.InitAdapters(); err != nil {
		log.Printf("Warning: Failed to initialize adapters: %v", err)
	}

	a.wsHub = handler.NewWebSocketHub()
	clientAdapter := client.NewAdapter()

	a.adminService = service.NewAdminService(
		cachedProviderRepo,
		cachedRouteRepo,
		cachedProjectRepo,
		cachedSessionRepo,
		cachedRetryConfigRepo,
		cachedRoutingStrategyRepo,
		proxyRequestRepo,
		attemptRepo,
		settingRepo,
		"",
		r,
	)

	a.antigravitySvc = NewAntigravityService(antigravityQuotaRepo, a.wsHub, a.adminService)

	exec := executor.NewExecutor(r, proxyRequestRepo, attemptRepo, cachedRetryConfigRepo, a.wsHub, "")

	proxyHandler := handler.NewProxyHandler(clientAdapter, exec, cachedSessionRepo)
	projectProxyHandler := handler.NewProjectProxyHandler(proxyHandler, cachedProjectRepo)

	mux := http.NewServeMux()

	mux.Handle("/v1/messages", proxyHandler)
	mux.Handle("/v1/chat/completions", proxyHandler)
	mux.Handle("/responses", proxyHandler)
	mux.Handle("/v1beta/models/", proxyHandler)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/ws", a.wsHub.HandleWebSocket)

	staticHandler := handler.NewStaticHandler()
	combinedHandler := handler.NewCombinedHandler(projectProxyHandler, staticHandler)
	mux.Handle("/", combinedHandler)

	a.httpServer = &http.Server{
		Addr:    ":19380",
		Handler: handler.LoggingMiddleware(mux),
	}

	go func() {
		log.Println("Internal HTTP server started on :19380")
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
}

func (a *App) startBackgroundTasks() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				before := len(cooldown.Default().GetAllCooldowns())
				cooldown.Default().CleanupExpired()
				after := len(cooldown.Default().GetAllCooldowns())

				if before != after {
					log.Printf("[Cooldown] Cleanup completed: removed %d expired entries", before-after)
				}
			case <-a.ctx.Done():
				return
			}
		}
	}()

	log.Println("[Cooldown] Background cleanup started (runs every 1 hour)")
}

func (a *App) emitLogEvent(message string) {
	if a.wsHub != nil {
		a.wsHub.BroadcastLog(message)
	}
	runtime.EventsEmit(a.ctx, "log", map[string]interface{}{
		"message": message,
		"time":    time.Now().Unix(),
	})
}

func (a *App) emitProxyRequestEvent(req interface{}) {
	runtime.EventsEmit(a.ctx, "proxyRequest", req)
}

func (a *App) emitAntigravityQuotaEvent(quota interface{}) {
	runtime.EventsEmit(a.ctx, "antigravityQuota", quota)
}

func (a *App) emitCooldownEvent(cooldown interface{}) {
	runtime.EventsEmit(a.ctx, "cooldown", cooldown)
}

func (a *App) emitFailureCountEvent(failureCount interface{}) {
	runtime.EventsEmit(a.ctx, "failureCount", failureCount)
}
