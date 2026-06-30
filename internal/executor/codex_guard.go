package executor

import (
	"log"
	"strings"

	"github.com/awsl-project/maxx/internal/codexguard"
	"github.com/awsl-project/maxx/internal/domain"
)

func (e *Executor) getCodexGuardConfig() codexguard.Config {
	cfg := codexguard.DefaultConfig()
	if e == nil || e.settingsRepo == nil {
		return cfg
	}

	raw, err := e.settingsRepo.Get(domain.SettingKeyCodexReasoningGuard)
	if err != nil {
		log.Printf("[Executor] failed to load %s setting, disabling guard: %v", domain.SettingKeyCodexReasoningGuard, err)
		return disabledCodexGuardConfig()
	}
	if strings.TrimSpace(raw) == "" {
		return cfg
	}

	parsed, err := codexguard.ParseConfigJSON([]byte(raw))
	if err != nil {
		log.Printf("[Executor] invalid %s setting, disabling guard: %v", domain.SettingKeyCodexReasoningGuard, err)
		return disabledCodexGuardConfig()
	}
	return parsed
}

func disabledCodexGuardConfig() codexguard.Config {
	cfg := codexguard.DefaultConfig()
	cfg.Enabled = false
	return cfg
}

func shouldRetryCodexGuard(err error, cfg codexguard.Config, guardFailures int) bool {
	return cfg.Enabled && codexguard.IsReasoningGuardError(err) && guardFailures < cfg.MaxAttempts
}
