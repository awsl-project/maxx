//go:build !windows

package desktop

func packagedWindowsApp() bool { return false }

func readAutoStartEnabled() (bool, error) { return false, nil }

func setAutoStartEnabled(bool) error { return nil }
