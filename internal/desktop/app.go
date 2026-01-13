package desktop

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Bowl42/maxx/internal/core"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type ServerStatus string

const (
	ServerStatusStopped ServerStatus = "stopped"
	ServerStatusRunning ServerStatus = "running"
)

type DesktopApp struct {
	ctx            context.Context
	server         *core.ManagedServer
	dbRepos         *core.DatabaseRepos
	components      *core.ServerComponents
	trayManager    *TrayManager
	dataDir        string
	serverPort     string
	instanceID      string
	serverStatus   ServerStatus
	trayMode       bool
	autoStartEnabled bool
}

func NewDesktopApp() (*DesktopApp, error) {
	dataDir := getWindowsDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	log.Printf("[Desktop] Data directory: %s", dataDir)

	dbConfig := &core.DatabaseConfig{
		DataDir: dataDir,
		DBPath:  filepath.Join(dataDir, "maxx.db"),
		LogPath: filepath.Join(dataDir, "maxx.log"),
	}

	dbRepos, err := core.InitializeDatabase(dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	instanceID := generateInstanceID()

	components, err := core.InitializeServerComponents(
		dbRepos,
		":9880",
		instanceID,
		filepath.Join(dataDir, "maxx.log"),
	)
	if err != nil {
		_ = core.CloseDatabase(dbRepos)
		return nil, fmt.Errorf("failed to initialize server components: %w", err)
	}

	app := &DesktopApp{
		dbRepos:          dbRepos,
		components:       components,
		dataDir:          dataDir,
		serverPort:       ":9880",
		instanceID:       instanceID,
		serverStatus:     ServerStatusStopped,
		trayMode:        true,
		autoStartEnabled: isAutoStartEnabled(),
	}

	return app, nil
}

func (a *DesktopApp) Startup(ctx context.Context) {
	a.ctx = ctx

	log.Println("[Desktop] ========== Application Startup ==========")
	log.Printf("[Desktop] Data directory: %s", a.dataDir)
	log.Printf("[Desktop] Instance ID: %s", a.instanceID)

	// Set Wails context on the broadcaster for event emission
	if a.components != nil && a.components.WailsBroadcaster != nil {
		log.Println("[Desktop] Setting Wails context on broadcaster")
		a.components.WailsBroadcaster.SetContext(ctx)
	}

	if err := a.StartServer(); err != nil {
		log.Printf("[Desktop] Failed to start server: %v", err)
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "启动失败",
			Message: fmt.Sprintf("无法启动 HTTP 服务器:\n%v", err),
		})
	}

	a.trayManager = NewTrayManager(a)

	log.Println("[Desktop] ========== Application Started ==========")
}

func (a *DesktopApp) Shutdown(ctx context.Context) {
	log.Println("[Desktop] ========== Application Shutdown ==========")

	if a.trayManager != nil {
		a.trayManager.Shutdown()
	}

	if err := a.StopServer(); err != nil {
		log.Printf("[Desktop] Failed to stop server: %v", err)
	}

	if err := core.CloseDatabase(a.dbRepos); err != nil {
		log.Printf("[Desktop] Failed to close database: %v", err)
	}

	log.Println("[Desktop] ========== Application Shutdown Complete ==========")
}

func (a *DesktopApp) DomReady(ctx context.Context) {
	log.Println("[Desktop] DOM ready")
}

func (a *DesktopApp) BeforeClose(ctx context.Context) (prevent bool) {
	log.Println("[Desktop] Window close requested")

	if a.trayManager != nil && a.trayManager.IsTrayMode() {
		log.Println("[Desktop] Hiding window to tray")
		runtime.Hide(ctx)
		runtime.EventsEmit(ctx, "window-hidden")
		return true
	}

	log.Println("[Desktop] Quitting application")
	return false
}

func (a *DesktopApp) StartServer() error {
	log.Println("[Desktop] Starting HTTP server...")

	if a.server != nil && a.server.IsRunning() {
		log.Println("[Desktop] Server already running")
		return nil
	}

	serverConfig := &core.ServerConfig{
		Addr:         a.serverPort,
		DataDir:      a.dataDir,
		InstanceID:   a.instanceID,
		Components:   a.components,
		ServeStatic:  false,
	}

	var err error
	a.server, err = core.NewManagedServer(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	if err := a.server.Start(a.ctx); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	a.serverStatus = ServerStatusRunning
	runtime.EventsEmit(a.ctx, "server-started", map[string]interface{}{
		"address": "http://localhost:9880",
		"port":    9880,
	})

	log.Println("[Desktop] Server started on :9880")
	return nil
}

func (a *DesktopApp) StopServer() error {
	log.Println("[Desktop] Stopping HTTP server...")

	if a.server == nil || !a.server.IsRunning() {
		log.Println("[Desktop] Server already stopped")
		return nil
	}

	if err := a.server.Stop(a.ctx); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	a.serverStatus = ServerStatusStopped
	runtime.EventsEmit(a.ctx, "server-stopped")

	log.Println("[Desktop] Server stopped")
	return nil
}

func (a *DesktopApp) RestartServer() error {
	log.Println("[Desktop] Restarting HTTP server...")

	if err := a.StopServer(); err != nil {
		return err
	}

	time.Sleep(500 * time.Millisecond)

	return a.StartServer()
}

func (a *DesktopApp) GetServerStatus() string {
	if a.server != nil && a.server.IsRunning() {
		return string(ServerStatusRunning)
	}
	return string(ServerStatusStopped)
}

func (a *DesktopApp) GetServerAddress() string {
	return "http://localhost:9880"
}

func (a *DesktopApp) OpenDataDir() error {
	dataDirPath := filepath.ToSlash(a.dataDir)
	runtime.BrowserOpenURL(a.ctx, "file://"+dataDirPath)
	return nil
}

func (a *DesktopApp) OpenLogFile() error {
	logPath := filepath.Join(a.dataDir, "maxx.log")
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(logPath))
	return nil
}

func (a *DesktopApp) CopyServerAddress() error {
	runtime.ClipboardSetText(a.ctx, "http://localhost:9880")
	runtime.EventsEmit(a.ctx, "address-copied")
	return nil
}

func (a *DesktopApp) SetTrayMode(enabled bool) error {
	a.trayMode = enabled
	log.Printf("[Desktop] Tray mode set to: %v", enabled)
	return nil
}

func (a *DesktopApp) IsTrayMode() bool {
	return a.trayMode
}

func (a *DesktopApp) SetAutoStart(enabled bool) error {
	log.Printf("[Desktop] Setting auto-start to: %v", enabled)
	if err := setAutoStart(enabled); err != nil {
		return fmt.Errorf("failed to set auto-start: %w", err)
	}
	a.autoStartEnabled = enabled
	return nil
}

func (a *DesktopApp) IsAutoStartEnabled() bool {
	return isAutoStartEnabled()
}

func (a *DesktopApp) ShowWindow() {
	runtime.Show(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	time.Sleep(100 * time.Millisecond)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
}

func (a *DesktopApp) HideWindow() {
	runtime.Hide(a.ctx)
}

func (a *DesktopApp) Quit() {
	log.Println("[Desktop] Quitting application")
	if a.server != nil {
		_ = a.StopServer()
	}
	runtime.Quit(a.ctx)
}

func getWindowsDataDir() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "maxx")
	}
	return filepath.Join(appData, "maxx")
}

func generateInstanceID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
}
