package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/core"
	"github.com/awsl-project/maxx/internal/version"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// DesktopConfig 桌面应用配置
type DesktopConfig struct {
	Port int `json:"port"` // HTTP 服务端口，默认 9880
}

// DefaultConfig 返回默认配置
func DefaultConfig() *DesktopConfig {
	return &DesktopConfig{
		Port: 9880,
	}
}

// loadConfig 从文件加载配置
func loadConfig(dataDir string) *DesktopConfig {
	configPath := filepath.Join(dataDir, "desktop.json")
	config := DefaultConfig()

	data, err := os.ReadFile(configPath)
	if err != nil {
		// 配置文件不存在，使用默认配置
		return config
	}

	if err := json.Unmarshal(data, config); err != nil {
		log.Printf("[Launcher] Failed to parse config: %v, using defaults", err)
		return DefaultConfig()
	}

	// 验证端口范围
	if config.Port < 1 || config.Port > 65535 {
		config.Port = 9880
	}

	return config
}

// saveConfig 保存配置到文件
func saveConfig(dataDir string, config *DesktopConfig) error {
	configPath := filepath.Join(dataDir, "desktop.json")

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// getDataDir 获取数据目录
func getDataDir() string {
	// 优先使用环境变量
	if dir := os.Getenv("MAXX_DATA_DIR"); dir != "" {
		return dir
	}

	// Windows: 使用 APPDATA
	appData := os.Getenv("APPDATA")
	if appData != "" {
		return filepath.Join(appData, "maxx")
	}

	// macOS/Linux: 使用 ~/.config/maxx
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "maxx")
}

// generateInstanceID 生成实例 ID
func generateInstanceID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
}

// ServerStatusInfo 服务器状态信息（暴露给前端）
type ServerStatusInfo struct {
	Ready       bool   `json:"Ready"`
	RedirectURL string `json:"RedirectURL,omitempty"` // 需要跳转的地址
	Error       string `json:"Error,omitempty"`
	Message     string `json:"Message,omitempty"` // 状态消息
}

// LauncherApp 启动器应用（简化版 DesktopApp）
// 只负责显示启动画面和启动 HTTP Server
type LauncherApp struct {
	ctx        context.Context
	server     *core.ManagedServer
	dbRepos    *core.DatabaseRepos
	components *core.ServerComponents
	dataDir    string
	serverPort string
	instanceID string
	config     *DesktopConfig

	// 状态
	mu          sync.RWMutex
	serverError error
	serverReady bool
	starting    bool
}

// NewLauncherApp 创建启动器应用
func NewLauncherApp() (*LauncherApp, error) {
	dataDir := getDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	log.Printf("[Launcher] Data directory: %s", dataDir)

	// 加载配置
	config := loadConfig(dataDir)
	log.Printf("[Launcher] Config loaded: port=%d", config.Port)

	app := &LauncherApp{
		dataDir:    dataDir,
		serverPort: fmt.Sprintf(":%d", config.Port),
		instanceID: generateInstanceID(),
		config:     config,
	}

	return app, nil
}

// Startup Wails 启动回调
func (a *LauncherApp) Startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("[Launcher] ========== Application Startup ==========")
	log.Printf("[Launcher] Data directory: %s", a.dataDir)
	log.Printf("[Launcher] Instance ID: %s", a.instanceID)

	// 清理可能占用的端口
	if a.config != nil {
		port := a.config.Port
		log.Printf("[Launcher] 检查端口 %d 是否被占用...", port)
		if err := TerminateProcessByPort(port); err != nil {
			log.Printf("[Launcher] 端口清理警告: %v", err)
		}
	}

	// 在后台 goroutine 中启动 HTTP Server
	go a.startServerAsync()
}

// startServerAsync 异步启动服务器
func (a *LauncherApp) startServerAsync() {
	a.mu.Lock()
	a.starting = true
	a.serverError = nil
	a.serverReady = false
	a.mu.Unlock()

	log.Println("[Launcher] Starting HTTP server in background...")

	// 初始化数据库
	dbConfig := &core.DatabaseConfig{
		DataDir: a.dataDir,
		DBPath:  filepath.Join(a.dataDir, "maxx.db"),
		LogPath: filepath.Join(a.dataDir, "maxx.log"),
	}

	dbRepos, err := core.InitializeDatabase(dbConfig)
	if err != nil {
		a.setError(fmt.Errorf("数据库初始化失败: %w", err))
		return
	}
	a.dbRepos = dbRepos

	// 初始化服务器组件
	components, err := core.InitializeServerComponents(
		dbRepos,
		a.serverPort,
		a.instanceID,
		filepath.Join(a.dataDir, "maxx.log"),
	)
	if err != nil {
		a.setError(fmt.Errorf("服务器组件初始化失败: %w", err))
		return
	}
	a.components = components

	// 设置 Wails context 用于事件广播
	if components.WailsBroadcaster != nil {
		components.WailsBroadcaster.SetContext(a.ctx)
	}

	// 创建并启动服务器（启用静态文件服务）
	serverConfig := &core.ServerConfig{
		Addr:        a.serverPort,
		DataDir:     a.dataDir,
		InstanceID:  a.instanceID,
		Components:  components,
		SettingRepo: dbRepos.SettingRepo,
		ServeStatic: true, // 关键：启用静态文件服务
	}

	server, err := core.NewManagedServer(serverConfig)
	if err != nil {
		a.setError(fmt.Errorf("服务器创建失败: %w", err))
		return
	}
	a.server = server

	if err := server.Start(a.ctx); err != nil {
		a.setError(fmt.Errorf("服务器启动失败: %w", err))
		return
	}

	// 等待服务器真正就绪（通过健康检查）
	if err := a.waitForServerReady(); err != nil {
		a.setError(err)
		return
	}

	a.mu.Lock()
	a.serverReady = true
	a.starting = false
	a.mu.Unlock()

	log.Printf("[Launcher] HTTP server started successfully on %s", a.serverPort)
	log.Println("[Launcher] ========== Server Ready ==========")
}

// waitForServerReady 等待服务器健康检查通过
func (a *LauncherApp) waitForServerReady() error {
	client := &http.Client{Timeout: 2 * time.Second}
	maxAttempts := 60 // 最多等待 6 秒

	for range maxAttempts {
		resp, err := client.Get(fmt.Sprintf("http://localhost%s/health", a.serverPort))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("服务器健康检查超时")
}

// setError 设置错误状态
func (a *LauncherApp) setError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.serverError = err
	a.starting = false
	log.Printf("[Launcher] Error: %v", err)
}

// CheckServerStatus 检查服务器状态（暴露给前端）
// 前端只需要调用这个函数，后端会返回是否需要跳转以及跳转地址
func (a *LauncherApp) CheckServerStatus() ServerStatusInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.serverError != nil {
		return ServerStatusInfo{
			Ready:   false,
			Error:   a.serverError.Error(),
			Message: "启动失败",
		}
	}

	if a.serverReady {
		return ServerStatusInfo{
			Ready:       true,
			RedirectURL: fmt.Sprintf("http://localhost%s", a.serverPort),
			Message:     "启动完成",
		}
	}

	return ServerStatusInfo{
		Ready:   false,
		Message: "正在启动服务...",
	}
}

// GetServerAddress 获取服务器地址（暴露给前端）
func (a *LauncherApp) GetServerAddress() string {
	return fmt.Sprintf("http://localhost%s", a.serverPort)
}

// GetVersion 获取版本信息（暴露给前端）
func (a *LauncherApp) GetVersion() string {
	return version.Full()
}

// RestartServer 重启服务器（暴露给前端）
func (a *LauncherApp) RestartServer() error {
	log.Println("[Launcher] Restarting server...")

	// 停止现有服务器
	if a.server != nil && a.server.IsRunning() {
		if err := a.server.Stop(a.ctx); err != nil {
			log.Printf("[Launcher] Failed to stop server: %v", err)
		}
	}

	// 关闭数据库
	if a.dbRepos != nil {
		if err := core.CloseDatabase(a.dbRepos); err != nil {
			log.Printf("[Launcher] Failed to close database: %v", err)
		}
		a.dbRepos = nil
	}

	// 更新端口（使用最新配置）
	if a.config != nil {
		a.serverPort = fmt.Sprintf(":%d", a.config.Port)
	}

	// 重置状态
	a.mu.Lock()
	a.serverError = nil
	a.serverReady = false
	a.server = nil
	a.components = nil
	a.mu.Unlock()

	// 重新启动
	go a.startServerAsync()
	return nil
}

// Quit 退出应用（暴露给前端）
func (a *LauncherApp) Quit() {
	log.Println("[Launcher] Quitting application...")

	// 停止服务器
	if a.server != nil {
		a.server.Stop(a.ctx)
	}

	// 关闭数据库
	if a.dbRepos != nil {
		core.CloseDatabase(a.dbRepos)
	}

	// 退出应用
	runtime.Quit(a.ctx)
}

// ShowWindow 显示窗口（供托盘调用）
func (a *LauncherApp) ShowWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
		runtime.WindowUnminimise(a.ctx)
	}
}

// HideWindow 隐藏窗口（供托盘调用）
func (a *LauncherApp) HideWindow() {
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
}

// Shutdown Wails 关闭回调
func (a *LauncherApp) Shutdown(ctx context.Context) {
	log.Println("[Launcher] ========== Application Shutdown ==========")

	if a.server != nil {
		if err := a.server.Stop(ctx); err != nil {
			log.Printf("[Launcher] Failed to stop server: %v", err)
		}
	}

	if a.dbRepos != nil {
		if err := core.CloseDatabase(a.dbRepos); err != nil {
			log.Printf("[Launcher] Failed to close database: %v", err)
		}
	}

	log.Println("[Launcher] ========== Application Shutdown Complete ==========")
}

// DomReady Wails DOM 就绪回调
func (a *LauncherApp) DomReady(ctx context.Context) {
	log.Println("[Launcher] DOM ready")
}

// BeforeClose Wails 关闭前回调
// 由平台特定文件实现：launcher_windows.go 和 launcher_other.go

// GetConfig 获取当前配置（暴露给前端）
func (a *LauncherApp) GetConfig() DesktopConfig {
	if a.config == nil {
		return *DefaultConfig()
	}
	return *a.config
}

// SaveConfig 保存配置（暴露给前端）
// 保存后需要重启应用才能生效
func (a *LauncherApp) SaveConfig(config DesktopConfig) error {
	// 验证端口范围
	if config.Port < 1 || config.Port > 65535 {
		return fmt.Errorf("端口必须在 1-65535 范围内")
	}

	// 保存到文件
	if err := saveConfig(a.dataDir, &config); err != nil {
		return err
	}

	a.mu.Lock()
	a.config = &config
	a.mu.Unlock()
	log.Printf("[Launcher] Config saved: port=%d", config.Port)

	return nil
}

// GetDataDir 获取数据目录（暴露给前端）
func (a *LauncherApp) GetDataDir() string {
	return a.dataDir
}

// OnSecondInstanceLaunch 当第二个实例尝试启动时触发
func (a *LauncherApp) OnSecondInstanceLaunch(data options.SecondInstanceData) {
	log.Println("[Launcher] 第二个实例尝试启动，激活已有窗口")

	// 如果窗口被最小化了，先还原
	if a.ctx != nil {
		runtime.WindowUnminimise(a.ctx)

		// 显示窗口
		runtime.WindowShow(a.ctx)

		// 强制将窗口置顶并聚焦
		runtime.WindowSetAlwaysOnTop(a.ctx, true)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
	}
}
