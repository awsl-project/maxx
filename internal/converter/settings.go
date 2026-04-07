package converter

import (
	"log"
	"sync"
)

// GlobalSettings holds converter-related global configuration.
type GlobalSettings struct {
	CodexInstructionsEnabled bool
}

var (
	globalSettingsMu   sync.RWMutex
	settingsGetterFunc func() (*GlobalSettings, error)
)

// SetGlobalSettingsGetter sets the function to retrieve global settings.
func SetGlobalSettingsGetter(getter func() (*GlobalSettings, error)) {
	globalSettingsMu.Lock()
	defer globalSettingsMu.Unlock()
	settingsGetterFunc = getter
}

// GetGlobalSettings retrieves the current global settings.
func GetGlobalSettings() *GlobalSettings {
	globalSettingsMu.RLock()
	defer globalSettingsMu.RUnlock()
	if settingsGetterFunc == nil {
		return nil
	}
	settings, err := settingsGetterFunc()
	if err != nil {
		log.Printf("[Converter] Failed to load global settings: %v; codex instructions processing will use default behavior", err)
		return nil
	}
	return settings
}
