//go:build windows

package desktop

import (
	"context"
	"log"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// BeforeClose Windows: 隐藏到托盘，不退出
func (a *LauncherApp) BeforeClose(ctx context.Context) bool {
	log.Println("[Launcher] Window close requested - hiding to tray")
	runtime.WindowHide(ctx)
	return true
}
