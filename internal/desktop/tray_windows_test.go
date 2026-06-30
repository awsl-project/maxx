//go:build windows

package desktop

import (
	"context"
	"testing"
)

func TestNewTrayManagerRegistersTrayQuitFunc(t *testing.T) {
	app := &LauncherApp{}
	manager := NewTrayManager(context.Background(), app)

	if manager.app != app {
		t.Fatalf("manager app = %#v, want original app", manager.app)
	}

	app.trayQuitMu.RLock()
	fn := app.trayQuit
	app.trayQuitMu.RUnlock()
	if fn == nil {
		t.Fatal("tray quit hook was not registered")
	}
}
