//go:build !windows

package desktop

import (
	"context"
	"log"
)

// BeforeClose 非 Windows: 允许正常退出
func (a *LauncherApp) BeforeClose(ctx context.Context) bool {
	log.Println("[Launcher] Window close requested")
	return false
}
