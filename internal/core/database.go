package core

import (
	"log"
	"os"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/client"
	"github.com/awsl-project/maxx/internal/adapter/provider/antigravity"
	"github.com/awsl-project/maxx/internal/adapter/provider/kiro"
	_ "github.com/awsl-project/maxx/internal/adapter/provider/custom"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/executor"
	"github.com/awsl-project/maxx/internal/handler"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/awsl-project/maxx/internal/service"
	"github.com/awsl-project/maxx/internal/waiter"
)

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	DataDir string
	DBPath  string
	LogPath string
}

// DatabaseRepos 包含所有数据库仓库
type DatabaseRepos struct {
	DB                       *sqlite.DB
	ProviderRepo             repository.ProviderRepository
	RouteRepo                repository.RouteRepository
	ProjectRepo              repository.ProjectRepository
	SessionRepo              repository.SessionRepository
	RetryConfigRepo          repository.RetryConfigRepository
	RoutingStrategyRepo       repository.RoutingStrategyRepository
	ProxyRequestRepo         repository.ProxyRequestRepository
	AttemptRepo              repository.ProxyUpstreamAttemptRepository
	SettingRepo              repository.SystemSettingRepository
	AntigravityQuotaRepo     repository.AntigravityQuotaRepository
	CooldownRepo             repository.CooldownRepository
	FailureCountRepo         repository.FailureCountRepository
	CachedProviderRepo        *cached.ProviderRepository
	CachedRouteRepo          *cached.RouteRepository
	CachedRetryConfigRepo    *cached.RetryConfigRepository
	CachedRoutingStrategyRepo *cached.RoutingStrategyRepository
	CachedSessionRepo        *cached.SessionRepository
	CachedProjectRepo        *cached.ProjectRepository
}

// ServerComponents 包含服务器运行所需的所有组件
type ServerComponents struct {
	Router              *router.Router
	WebSocketHub        *handler.WebSocketHub
	WailsBroadcaster    *event.WailsBroadcaster
	Executor            *executor.Executor
	ClientAdapter       *client.Adapter
	AdminService        *service.AdminService
	ProxyHandler        *handler.ProxyHandler
	AdminHandler        *handler.AdminHandler
	AntigravityHandler  *handler.AntigravityHandler
	KiroHandler         *handler.KiroHandler
	ProjectProxyHandler *handler.ProjectProxyHandler
}

// InitializeDatabase 初始化数据库和所有仓库
func InitializeDatabase(config *DatabaseConfig) (*DatabaseRepos, error) {
	log.Printf("[Core] Initializing database: %s", config.DBPath)

	db, err := sqlite.NewDB(config.DBPath)
	if err != nil {
		return nil, err
	}

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

	log.Printf("[Core] Creating cached repositories")

	cachedProviderRepo := cached.NewProviderRepository(providerRepo)
	cachedRouteRepo := cached.NewRouteRepository(routeRepo)
	cachedRetryConfigRepo := cached.NewRetryConfigRepository(retryConfigRepo)
	cachedRoutingStrategyRepo := cached.NewRoutingStrategyRepository(routingStrategyRepo)
	cachedSessionRepo := cached.NewSessionRepository(sessionRepo)
	cachedProjectRepo := cached.NewProjectRepository(projectRepo)

	repos := &DatabaseRepos{
		DB:                       db,
		ProviderRepo:             providerRepo,
		RouteRepo:                routeRepo,
		ProjectRepo:              projectRepo,
		SessionRepo:              sessionRepo,
		RetryConfigRepo:          retryConfigRepo,
		RoutingStrategyRepo:       routingStrategyRepo,
		ProxyRequestRepo:         proxyRequestRepo,
		AttemptRepo:              attemptRepo,
		SettingRepo:              settingRepo,
		AntigravityQuotaRepo:     antigravityQuotaRepo,
		CooldownRepo:             cooldownRepo,
		FailureCountRepo:         failureCountRepo,
		CachedProviderRepo:        cachedProviderRepo,
		CachedRouteRepo:          cachedRouteRepo,
		CachedRetryConfigRepo:    cachedRetryConfigRepo,
		CachedRoutingStrategyRepo: cachedRoutingStrategyRepo,
		CachedSessionRepo:        cachedSessionRepo,
		CachedProjectRepo:        cachedProjectRepo,
	}

	log.Printf("[Core] Database initialized successfully")
	return repos, nil
}

// InitializeServerComponents 初始化服务器运行所需的所有组件
func InitializeServerComponents(
	repos *DatabaseRepos,
	addr string,
	instanceID string,
	logPath string,
) (*ServerComponents, error) {
	log.Printf("[Core] Initializing server components")

	log.Printf("[Core] Initializing cooldown manager with database persistence")
	cooldown.Default().SetRepository(repos.CooldownRepo)
	cooldown.Default().SetFailureCountRepository(repos.FailureCountRepo)
	if err := cooldown.Default().LoadFromDatabase(); err != nil {
		log.Printf("[Core] Warning: Failed to load cooldowns from database: %v", err)
	}

	log.Printf("[Core] Marking stale requests as failed")
	if count, err := repos.ProxyRequestRepo.MarkStaleAsFailed(instanceID); err != nil {
		log.Printf("[Core] Warning: Failed to mark stale requests: %v", err)
	} else if count > 0 {
		log.Printf("[Core] Marked %d stale requests as failed", count)
	}

	log.Printf("[Core] Loading cached data")
	if err := repos.CachedProviderRepo.Load(); err != nil {
		log.Printf("[Core] Warning: Failed to load providers cache: %v", err)
	}
	if err := repos.CachedRouteRepo.Load(); err != nil {
		log.Printf("[Core] Warning: Failed to load routes cache: %v", err)
	}
	if err := repos.CachedRetryConfigRepo.Load(); err != nil {
		log.Printf("[Core] Warning: Failed to load retry configs cache: %v", err)
	}
	if err := repos.CachedRoutingStrategyRepo.Load(); err != nil {
		log.Printf("[Core] Warning: Failed to load routing strategies cache: %v", err)
	}
	if err := repos.CachedProjectRepo.Load(); err != nil {
		log.Printf("[Core] Warning: Failed to load projects cache: %v", err)
	}

	log.Printf("[Core] Creating router")
	r := router.NewRouter(
		repos.CachedRouteRepo,
		repos.CachedProviderRepo,
		repos.CachedRoutingStrategyRepo,
		repos.CachedRetryConfigRepo,
		repos.CachedProjectRepo,
	)

	log.Printf("[Core] Initializing provider adapters")
	if err := r.InitAdapters(); err != nil {
		log.Printf("[Core] Warning: Failed to initialize adapters: %v", err)
	}

	log.Printf("[Core] Starting cooldown cleanup goroutine")
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			before := len(cooldown.Default().GetAllCooldowns())
			cooldown.Default().CleanupExpired()
			after := len(cooldown.Default().GetAllCooldowns())

			if before != after {
				log.Printf("[Core] Cooldown cleanup completed: removed %d expired entries", before-after)
			}
		}
	}()

	log.Printf("[Core] Creating WebSocket hub")
	wsHub := handler.NewWebSocketHub()

	log.Printf("[Core] Creating Wails broadcaster (wraps WebSocket hub)")
	wailsBroadcaster := event.NewWailsBroadcaster(wsHub)

	log.Printf("[Core] Setting up log output to broadcast via WebSocket")
	logWriter := handler.NewWebSocketLogWriter(wsHub, os.Stdout, logPath)
	log.SetOutput(logWriter)

	log.Printf("[Core] Creating project waiter")
	projectWaiter := waiter.NewProjectWaiter(repos.CachedSessionRepo, repos.SettingRepo, wailsBroadcaster)

	log.Printf("[Core] Creating executor")
	exec := executor.NewExecutor(
		r,
		repos.ProxyRequestRepo,
		repos.AttemptRepo,
		repos.CachedRetryConfigRepo,
		repos.CachedSessionRepo,
		wailsBroadcaster,
		projectWaiter,
		instanceID,
	)

	log.Printf("[Core] Creating client adapter")
	clientAdapter := client.NewAdapter()

	log.Printf("[Core] Creating admin service")
	adminService := service.NewAdminService(
		repos.CachedProviderRepo,
		repos.CachedRouteRepo,
		repos.ProjectRepo,
		repos.CachedSessionRepo,
		repos.CachedRetryConfigRepo,
		repos.CachedRoutingStrategyRepo,
		repos.ProxyRequestRepo,
		repos.AttemptRepo,
		repos.SettingRepo,
		addr,
		r,
	)

	log.Printf("[Core] Initializing Antigravity global settings getter")
	antigravity.SetGlobalSettingsGetter(func() (*antigravity.GlobalSettings, error) {
		// Read model mapping rules from database
		rulesJSON, _ := repos.SettingRepo.Get(domain.SettingKeyAntigravityModelMapping)
		rules, err := antigravity.ParseModelMappingRules(rulesJSON)
		if err != nil {
			return nil, err
		}

		return &antigravity.GlobalSettings{
			ModelMappingRules: rules,
		}, nil
	})

	log.Printf("[Core] Initializing Kiro global settings getter")
	kiro.SetGlobalSettingsGetter(func() (*kiro.GlobalSettings, error) {
		// Read model mapping rules from database
		rulesJSON, _ := repos.SettingRepo.Get(domain.SettingKeyKiroModelMapping)
		rules, err := kiro.ParseModelMappingRules(rulesJSON)
		if err != nil {
			return nil, err
		}

		return &kiro.GlobalSettings{
			ModelMappingRules: rules,
		}, nil
	})

	log.Printf("[Core] Creating handlers")
	proxyHandler := handler.NewProxyHandler(clientAdapter, exec, repos.CachedSessionRepo)
	adminHandler := handler.NewAdminHandler(adminService, logPath)
	antigravityHandler := handler.NewAntigravityHandler(adminService, repos.AntigravityQuotaRepo, wailsBroadcaster)
	kiroHandler := handler.NewKiroHandler(adminService)
	projectProxyHandler := handler.NewProjectProxyHandler(proxyHandler, repos.CachedProjectRepo)

	components := &ServerComponents{
		Router:              r,
		WebSocketHub:        wsHub,
		WailsBroadcaster:    wailsBroadcaster,
		Executor:            exec,
		ClientAdapter:       clientAdapter,
		AdminService:        adminService,
		ProxyHandler:        proxyHandler,
		AdminHandler:        adminHandler,
		AntigravityHandler:  antigravityHandler,
		KiroHandler:         kiroHandler,
		ProjectProxyHandler: projectProxyHandler,
	}

	log.Printf("[Core] Server components initialized successfully")
	return components, nil
}

// CloseDatabase 关闭数据库连接
func CloseDatabase(repos *DatabaseRepos) error {
	if repos != nil && repos.DB != nil {
		return repos.DB.Close()
	}
	return nil
}
