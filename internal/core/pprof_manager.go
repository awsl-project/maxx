package core

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

// PprofConfig pprof 配置
type PprofConfig struct {
	Enabled  bool
	Port     int
	Password string
}

// PprofManager 管理 pprof 服务的启停
type PprofManager struct {
	settingRepo repository.SystemSettingRepository
	server      *http.Server
	mu          sync.RWMutex
	isRunning   bool
	config      *PprofConfig
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewPprofManager 创建 pprof 管理器
func NewPprofManager(settingRepo repository.SystemSettingRepository) *PprofManager {
	return &PprofManager{
		settingRepo: settingRepo,
		config:      &PprofConfig{},
	}
}

// loadConfig 从数据库加载配置
func (m *PprofManager) loadConfig() (*PprofConfig, error) {
	config := &PprofConfig{
		Enabled:  false,
		Port:     6060,
		Password: "",
	}

	// 读取是否启用
	enabledStr, err := m.settingRepo.Get(domain.SettingKeyEnablePprof)
	if err != nil {
		return nil, fmt.Errorf("failed to get enable_pprof setting: %w", err)
	}
	if enabledStr != "" {
		config.Enabled = enabledStr == "true"
	}

	// 读取端口
	portStr, err := m.settingRepo.Get(domain.SettingKeyPprofPort)
	if err != nil {
		return nil, fmt.Errorf("failed to get pprof_port setting: %w", err)
	}
	if portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil && port > 0 && port <= 65535 {
			config.Port = port
		}
	}

	// 读取密码
	password, err := m.settingRepo.Get(domain.SettingKeyPprofPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to get pprof_password setting: %w", err)
	}
	config.Password = password

	return config, nil
}

// Start 启动 pprof 管理器（读取配置并启动服务）
func (m *PprofManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 加载配置
	config, err := m.loadConfig()
	if err != nil {
		log.Printf("[Pprof] Failed to load config: %v", err)
		return err
	}

	m.config = config
	m.ctx, m.cancel = context.WithCancel(ctx)

	// 如果启用，则启动服务
	if config.Enabled {
		return m.startServerLocked()
	}

	log.Printf("[Pprof] Pprof is disabled in system settings")
	return nil
}

// Stop 停止 pprof 管理器
func (m *PprofManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}

	return m.stopServerLocked(ctx)
}

// ReloadPprofConfig 重新加载配置并重启服务（支持动态修改）
func (m *PprofManager) ReloadPprofConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 加载新配置
	newConfig, err := m.loadConfig()
	if err != nil {
		log.Printf("[Pprof] Failed to reload config: %v", err)
		return err
	}

	// 检查配置是否变化
	configChanged := m.config.Enabled != newConfig.Enabled ||
		m.config.Port != newConfig.Port ||
		m.config.Password != newConfig.Password

	if !configChanged {
		log.Printf("[Pprof] Config unchanged, skip reload")
		return nil
	}

	log.Printf("[Pprof] Config changed, reloading...")
	log.Printf("[Pprof] Old config: enabled=%v, port=%d", m.config.Enabled, m.config.Port)
	log.Printf("[Pprof] New config: enabled=%v, port=%d", newConfig.Enabled, newConfig.Port)

	// 停止旧服务
	if m.isRunning {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := m.stopServerLocked(shutdownCtx); err != nil {
			log.Printf("[Pprof] Failed to stop old server: %v", err)
		}
	}

	m.config = newConfig

	// 启动新服务（如果启用）
	if newConfig.Enabled {
		return m.startServerLocked()
	}

	log.Printf("[Pprof] Pprof disabled after reload")
	return nil
}

// startServerLocked 启动 pprof 服务（需要持有锁）
func (m *PprofManager) startServerLocked() error {
	if m.isRunning {
		return fmt.Errorf("pprof server already running")
	}

	addr := fmt.Sprintf("localhost:%d", m.config.Port)

	// 先尝试绑定端口以验证是否可用
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[Pprof] Failed to bind to %s: %v", addr, err)
		return fmt.Errorf("failed to bind pprof server to %s: %w", addr, err)
	}

	// 创建独立的 pprof mux，避免暴露主应用的其他路由
	pprofMux := http.NewServeMux()
	pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
	pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// 创建带密码保护的 handler
	var handler http.Handler = pprofMux
	if m.config.Password != "" {
		// 在创建中间件时捕获密码值,避免在请求处理时无锁读取 m.config
		handler = m.basicAuthMiddleware(pprofMux, m.config.Password)
	}

	m.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// 端口绑定成功,设置运行状态
	m.isRunning = true

	// 在启动 goroutine 前复制需要的配置值和 server 实例，避免 goroutine 中访问 m.config 和 m.server 造成数据竞争
	hasPassword := m.config.Password != ""
	srv := m.server

	go func() {
		log.Printf("[Pprof] Starting pprof server on %s", addr)
		if hasPassword {
			log.Printf("[Pprof] Password protection enabled")
		}
		log.Printf("[Pprof] Access pprof at http://%s/debug/pprof/", addr)

		if srv != nil {
			if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
				log.Printf("[Pprof] Server error: %v", err)
				// 服务器异常退出,更新运行状态
				m.mu.Lock()
				m.isRunning = false
				m.mu.Unlock()
			}
		}
	}()

	return nil
}

// stopServerLocked 停止 pprof 服务（需要持有锁）
func (m *PprofManager) stopServerLocked(ctx context.Context) error {
	if !m.isRunning || m.server == nil {
		return nil
	}

	log.Printf("[Pprof] Stopping pprof server")

	if err := m.server.Shutdown(ctx); err != nil {
		log.Printf("[Pprof] Graceful shutdown failed: %v, forcing close", err)
		if closeErr := m.server.Close(); closeErr != nil {
			log.Printf("[Pprof] Force close error: %v", closeErr)
		}
	}

	m.server = nil
	m.isRunning = false
	log.Printf("[Pprof] Pprof server stopped")
	return nil
}

// basicAuthMiddleware 添加基本认证中间件
// 在创建时捕获密码值,避免在请求处理时访问 m.config 导致数据竞争
func (m *PprofManager) basicAuthMiddleware(next http.Handler, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, reqPassword, ok := r.BasicAuth()

		// 使用 "pprof" 作为用户名，密码从参数获取
		// 使用 subtle.ConstantTimeCompare 防止时序攻击
		validUsername := subtle.ConstantTimeCompare([]byte(username), []byte("pprof")) == 1
		validPassword := subtle.ConstantTimeCompare([]byte(reqPassword), []byte(password)) == 1

		if !ok || !validUsername || !validPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="pprof"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// IsRunning 检查 pprof 服务是否运行中
func (m *PprofManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

// GetConfig 获取当前配置
func (m *PprofManager) GetConfig() PprofConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.config
}
