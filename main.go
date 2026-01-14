package main

import (
	"embed"
	"log"

	"github.com/Bowl42/maxx-next/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:web/dist
var assets embed.FS

func main() {
	err := wails.Run(&options.App{
		Title:  "Maxx",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.Instance.OnStartup,
		OnShutdown:       app.Instance.OnShutdown,
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  false,
				HideTitleBar:               false,
				FullSizeContent:            false,
				UseToolbar:                 false,
				HideToolbarSeparator:       true,
			},
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "Maxx",
				Message: "Maxx - AI API Gateway & Proxy\n\n© 2025 Bowl42",
			},
		},
	})

	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}
