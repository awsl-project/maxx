package core

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/awsl-project/maxx/internal/handler"
	"github.com/awsl-project/maxx/internal/repository"
)

// Graceful shutdown configuration
const (
	// GracefulShutdownTimeout is the maximum time to wait for active requests
	GracefulShutdownTimeout = 2 * time.Minute
	// HTTPShutdownTimeout is the timeout for HTTP server shutdown after requests complete
	HTTPShutdownTimeout = 5 * time.Second
)

// ServerConfig 服务器配置
type ServerConfig struct {
	Addr        string
	DataDir     string
	InstanceID  string
	Components  *ServerComponents
	SettingRepo repository.SystemSettingRepository
	ServeStatic bool
}

// ManagedServer 可管理的服务器（支持启动/停止）
type ManagedServer struct {
	config       *ServerConfig
	httpServer   *http.Server
	pprofManager *PprofManager
	mux          *http.ServeMux
	isRunning    bool
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewManagedServer 创建可管理的服务器
func NewManagedServer(config *ServerConfig) (*ManagedServer, error) {
	log.Printf("[Server] Creating managed server on %s", config.Addr)

	s := &ManagedServer{
		config:    config,
		isRunning: false,
	}

	// 从 Components 中获取 PprofManager（如果有）
	if config.Components != nil && config.Components.PprofManager != nil {
		s.pprofManager = config.Components.PprofManager
		log.Printf("[Server] Using pprof manager from components")
	} else if config.SettingRepo != nil {
		// 向后兼容：如果 Components 中没有，则自己创建
		s.pprofManager = NewPprofManager(config.SettingRepo)
		log.Printf("[Server] Created new pprof manager")
	}

	s.mux = s.setupRoutes()

	log.Printf("[Server] Managed server created")
	return s, nil
}

// setupRoutes 设置所有路由
func (s *ManagedServer) setupRoutes() *http.ServeMux {
	log.Printf("[Server] Setting up routes")
	mux := http.NewServeMux()

	components := s.config.Components

	// API routes under /api prefix (Go 1.22+ enhanced routing)
	mux.Handle("/api/admin/", http.StripPrefix("/api", components.AdminHandler))
	mux.Handle("/api/antigravity/", http.StripPrefix("/api", components.AntigravityHandler))
	mux.Handle("/api/kiro/", http.StripPrefix("/api", components.KiroHandler))

	mux.Handle("/v1/messages", components.ProxyHandler)
	mux.Handle("/v1/chat/completions", components.ProxyHandler)
	mux.Handle("/responses", components.ProxyHandler)
	mux.Handle("/v1beta/models/", components.ProxyHandler)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/ws", components.WebSocketHub.HandleWebSocket)

	if s.config.ServeStatic {
		staticHandler := handler.NewStaticHandler()
		combinedHandler := handler.NewCombinedHandler(components.ProjectProxyHandler, staticHandler)
		mux.Handle("/", combinedHandler)
		log.Printf("[Server] Static file serving enabled")
	} else {
		mux.Handle("/", components.ProjectProxyHandler)
		log.Printf("[Server] Static file serving disabled (Wails mode)")
	}

	log.Printf("[Server] Routes configured")
	return mux
}

// Start 启动服务器
func (s *ManagedServer) Start(ctx context.Context) error {
	if s.isRunning {
		log.Printf("[Server] Server already running")
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	s.httpServer = &http.Server{
		Addr:     s.config.Addr,
		Handler:  s.mux,
		ErrorLog: nil,
	}

	go func() {
		log.Printf("[Server] Starting HTTP server on %s", s.config.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Server] Server error: %v", err)
		}
	}()

	// 启动 pprof 管理器
	if s.pprofManager != nil {
		if err := s.pprofManager.Start(s.ctx); err != nil {
			log.Printf("[Server] Failed to start pprof manager: %v", err)
		}
	}

	s.isRunning = true
	log.Printf("[Server] Server started successfully")
	return nil
}

// Stop 停止服务器
func (s *ManagedServer) Stop(ctx context.Context) error {
	if !s.isRunning {
		log.Printf("[Server] Server already stopped")
		return nil
	}

	log.Printf("[Server] Stopping HTTP server on %s", s.config.Addr)

	// Step 1: Wait for active proxy requests to complete (graceful shutdown)
	if s.config.Components != nil && s.config.Components.RequestTracker != nil {
		tracker := s.config.Components.RequestTracker
		activeCount := tracker.ActiveCount()

		if activeCount > 0 {
			log.Printf("[Server] Waiting for %d active proxy requests to complete...", activeCount)

			completed := tracker.GracefulShutdown(GracefulShutdownTimeout)
			if !completed {
				log.Printf("[Server] Graceful shutdown timeout, some requests may be interrupted")
			} else {
				log.Printf("[Server] All proxy requests completed successfully")
			}
		} else {
			// Mark as shutting down to reject new requests
			tracker.GracefulShutdown(0)
			log.Printf("[Server] No active proxy requests")
		}
	}

	// Step 2: Shutdown HTTP server (with shorter timeout since requests should be done)
	shutdownCtx, cancel := context.WithTimeout(ctx, HTTPShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Server] HTTP server graceful shutdown failed: %v, forcing close", err)
		// 强制关闭
		if closeErr := s.httpServer.Close(); closeErr != nil {
			log.Printf("[Server] Force close error: %v", closeErr)
		}
	}

	// 停止 pprof 管理器
	if s.pprofManager != nil {
		pprofCtx, pprofCancel := context.WithTimeout(ctx, 2*time.Second)
		defer pprofCancel()
		if err := s.pprofManager.Stop(pprofCtx); err != nil {
			log.Printf("[Server] Failed to stop pprof manager: %v", err)
		}
	}

	if s.cancel != nil {
		s.cancel()
	}

	s.isRunning = false
	log.Printf("[Server] Server stopped successfully")
	return nil
}

// IsRunning 检查服务器是否在运行
func (s *ManagedServer) IsRunning() bool {
	return s.isRunning
}

// GetAddr 获取服务器监听地址
func (s *ManagedServer) GetAddr() string {
	return s.config.Addr
}

// GetDataDir 获取数据目录
func (s *ManagedServer) GetDataDir() string {
	return s.config.DataDir
}

// GetInstanceID 获取实例 ID
func (s *ManagedServer) GetInstanceID() string {
	return s.config.InstanceID
}

// GetComponents 获取服务器组件
func (s *ManagedServer) GetComponents() *ServerComponents {
	return s.config.Components
}

// ReloadPprofConfig 重新加载 pprof 配置（支持动态修改）
func (s *ManagedServer) ReloadPprofConfig() error {
	if s.pprofManager == nil {
		return nil
	}
	return s.pprofManager.ReloadPprofConfig()
}
