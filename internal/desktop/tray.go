package desktop

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed icon.ico
var iconData []byte

// TrayManager 管理系统托盘
type TrayManager struct {
	ctx              context.Context
	app              *LauncherApp
	menuShow         *systray.MenuItem
	menuServerStatus *systray.MenuItem
	menuServerAddr   *systray.MenuItem
	menuSettings     *systray.MenuItem
	menuRestart      *systray.MenuItem
	menuQuit         *systray.MenuItem
}

// NewTrayManager 创建托盘管理器
func NewTrayManager(ctx context.Context, app *LauncherApp) *TrayManager {
	return &TrayManager{
		ctx: ctx,
		app: app,
	}
}

// Start 启动托盘
func (t *TrayManager) Start() {
	systray.Run(t.onReady, t.onExit)
}

// onReady 托盘就绪回调
func (t *TrayManager) onReady() {
	log.Println("[Tray] Initializing system tray...")

	// 设置托盘图标和提示
	systray.SetIcon(iconData)
	systray.SetTitle("Maxx")
	systray.SetTooltip("Maxx - AI API Proxy Gateway")

	// 创建菜单项
	t.menuShow = systray.AddMenuItem("显示窗口", "显示主窗口")
	systray.AddSeparator()

	// 服务器状态（只读）
	t.menuServerStatus = systray.AddMenuItem("服务器状态: 检查中...", "服务器运行状态")
	t.menuServerStatus.Disable()

	t.menuServerAddr = systray.AddMenuItem("服务器地址: -", "服务器监听地址")
	t.menuServerAddr.Disable()

	systray.AddSeparator()

	// 操作菜单
	t.menuSettings = systray.AddMenuItem("打开设置", "打开设置页面")
	t.menuRestart = systray.AddMenuItem("重启服务器", "重启 HTTP 服务器")

	systray.AddSeparator()

	t.menuQuit = systray.AddMenuItem("退出", "退出应用")

	// 初始更新状态
	t.UpdateStatus()

	// 启动菜单事件监听
	go t.handleMenuEvents()
}

// onExit 托盘退出回调
func (t *TrayManager) onExit() {
	log.Println("[Tray] System tray exited")
}

// handleMenuEvents 处理菜单事件
func (t *TrayManager) handleMenuEvents() {
	for {
		select {
		case <-t.menuShow.ClickedCh:
			log.Println("[Tray] Show window clicked")
			t.showWindow()

		case <-t.menuSettings.ClickedCh:
			log.Println("[Tray] Settings clicked")
			t.openSettings()

		case <-t.menuRestart.ClickedCh:
			log.Println("[Tray] Restart server clicked")
			t.restartServer()

		case <-t.menuQuit.ClickedCh:
			log.Println("[Tray] Quit clicked")
			t.quit()
			return
		}
	}
}

// showWindow 显示窗口
func (t *TrayManager) showWindow() {
	runtime.WindowShow(t.ctx)
	runtime.WindowUnminimise(t.ctx)
}

// openSettings 打开设置页面
func (t *TrayManager) openSettings() {
	runtime.WindowShow(t.ctx)
	runtime.WindowUnminimise(t.ctx)
	// 通过 JS 导航到设置页面
	runtime.WindowExecJS(t.ctx, `window.location.href = 'wails://wails/index.html?page=settings';`)
}

// restartServer 重启服务器
func (t *TrayManager) restartServer() {
	if t.app != nil {
		log.Println("[Tray] Restarting server...")
		t.app.RestartServer()
		// 延迟更新状态
		go func() {
			// 等待服务器重启
			t.UpdateStatus()
		}()
	}
}

// quit 退出应用
func (t *TrayManager) quit() {
	log.Println("[Tray] Quitting application...")
	if t.app != nil {
		t.app.Quit()
	}
	systray.Quit()
}

// UpdateStatus 更新托盘菜单状态
func (t *TrayManager) UpdateStatus() {
	if t.app == nil {
		return
	}

	status := t.app.CheckServerStatus()

	// 更新服务器状态
	if status.Ready {
		t.menuServerStatus.SetTitle("服务器状态: 运行中")
	} else {
		t.menuServerStatus.SetTitle("服务器状态: 已停止")
	}

	// 更新服务器地址
	addr := t.app.GetServerAddress()
	if addr != "" {
		t.menuServerAddr.SetTitle(fmt.Sprintf("服务器地址: %s", addr))
	} else {
		t.menuServerAddr.SetTitle("服务器地址: -")
	}
}
