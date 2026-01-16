package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	goruntime "runtime"

	"github.com/awsl-project/maxx/internal/desktop"
	"github.com/awsl-project/maxx/internal/handler"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:launcher
var assets embed.FS

//go:embed all:web/dist
var webDistAssets embed.FS

// 保存 app context 用于菜单回调
var appCtx context.Context

func main() {
	// Set embedded static files for HTTP server
	if subFS, err := fs.Sub(webDistAssets, "web/dist"); err == nil {
		handler.StaticFS = subFS
	}

	// Create desktop app instance
	app, err := desktop.NewLauncherApp()
	if err != nil {
		log.Fatal("Failed to initialize desktop app:", err)
	}

	// 初始化托盘（在 goroutine 中运行，避免阻塞主线程）
	go func() {
		// 等待 app context 初始化
		for appCtx == nil {
			// 等待 OnStartup 设置 appCtx
		}
		tray := desktop.NewTrayManager(appCtx, app)
		tray.Start()
	}()

	// Create application menu (only for macOS)
	var appMenu *menu.Menu
	if goruntime.GOOS == "darwin" {
		appMenu = menu.NewMenu()

		// macOS App Menu (Maxx)
		appMenu.Append(menu.AppMenu())

		// File Menu
		fileMenu := appMenu.AddSubmenu("File")
		fileMenu.AddText("Home", keys.CmdOrCtrl("h"), func(_ *menu.CallbackData) {
			if appCtx != nil {
				runtime.WindowExecJS(appCtx, `window.location.href = 'wails://wails/index.html';`)
			}
		})
		fileMenu.AddText("Settings", keys.CmdOrCtrl(","), func(_ *menu.CallbackData) {
			if appCtx != nil {
				runtime.WindowExecJS(appCtx, `window.location.href = 'wails://wails/index.html?page=settings';`)
			}
		})
		fileMenu.AddSeparator()
		fileMenu.AddText("Quit", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
			if appCtx != nil {
				runtime.Quit(appCtx)
			}
		})

		// Edit Menu (for copy/paste support)
		appMenu.Append(menu.EditMenu())
	}

	// Run Wails application
	err = wails.Run(&options.App{
		Title:              "Maxx",
		Width:              1280,
		Height:             800,
		MinWidth:           1024,
		MinHeight:          768,
		HideWindowOnClose:  true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup: func(ctx context.Context) {
			appCtx = ctx
			app.Startup(ctx)
		},
		OnDomReady:    app.DomReady,
		OnBeforeClose: app.BeforeClose,
		OnShutdown:    app.Shutdown,
		Bind: []interface{}{
			app,
		},
		Menu: appMenu,
		// 启用 DevTools 方便调试
		Debug: options.Debug{
			OpenInspectorOnStartup: false,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: false,
				HideTitle:                  false,
				HideTitleBar:               false,
				FullSizeContent:            false,
				UseToolbar:                 false,
				HideToolbarSeparator:       true,
			},
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			About: &mac.AboutInfo{
				Title:   "Maxx",
				Message: "AI API Proxy Gateway\n© 2024 awsl-project",
			},
		},
	})

	if err != nil {
		log.Fatal("Error:", err)
	}
}
