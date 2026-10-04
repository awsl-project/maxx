//go:build windows

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const autoStartRegistryName = "Maxx"
const autoStartRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func packagedWindowsApp() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	name := strings.ToLower(filepath.Base(exe))
	if name != "maxx.exe" {
		return false
	}
	return os.Getenv("WAILS_VITE_DEVSERVER_URL") == ""
}

func autoStartCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%q", exe), nil
}

func readAutoStartEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	defer key.Close()

	value, _, err := key.GetStringValue(autoStartRegistryName)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}

	want, err := autoStartCommand()
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value), want), nil
}

func setAutoStartEnabled(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, autoStartRegistryPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue(autoStartRegistryName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}

	value, err := autoStartCommand()
	if err != nil {
		return err
	}
	return key.SetStringValue(autoStartRegistryName, value)
}
