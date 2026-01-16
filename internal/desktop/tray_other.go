//go:build !windows

package desktop

import "context"

// TrayManager stub for non-Windows platforms
type TrayManager struct{}

// NewTrayManager creates a no-op tray manager
func NewTrayManager(ctx context.Context, app *LauncherApp) *TrayManager {
	return &TrayManager{}
}

// Start is a no-op on non-Windows platforms
func (t *TrayManager) Start() {}

// UpdateStatus is a no-op on non-Windows platforms
func (t *TrayManager) UpdateStatus() {}
