package executor

import (
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

const (
	flowKeyStreamFirstEventTimeout = "stream_first_event_timeout"
	flowKeyStreamIdleTimeout       = "stream_idle_timeout"

	defaultStreamFirstEventTimeout = 20 * time.Second
	defaultStreamIdleTimeout       = 45 * time.Second
)

func (e *Executor) openAIChatStreamTimeoutsEnabled() bool {
	if e == nil || e.settingsRepo == nil {
		return false
	}
	value, err := e.settingsRepo.Get(domain.SettingKeyOpenAIChatStreamTimeoutsEnabled)
	if err != nil {
		return false
	}
	return strings.TrimSpace(strings.ToLower(value)) == "true"
}

func (e *Executor) streamFirstEventTimeout() time.Duration {
	return e.streamTimeoutSetting(domain.SettingKeyOpenAIChatStreamFirstEventTimeoutMS, defaultStreamFirstEventTimeout)
}

func (e *Executor) streamIdleTimeout() time.Duration {
	return e.streamTimeoutSetting(domain.SettingKeyOpenAIChatStreamIdleTimeoutMS, defaultStreamIdleTimeout)
}

func (e *Executor) streamTimeoutSetting(key string, fallback time.Duration) time.Duration {
	if e == nil || e.settingsRepo == nil {
		return fallback
	}
	value, err := e.settingsRepo.Get(key)
	if err != nil {
		return fallback
	}
	milliseconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || milliseconds < 1000 || milliseconds > 600000 {
		return fallback
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func (e *Executor) shouldApplyOpenAIChatStreamTimeouts(originalClientType domain.ClientType, requestURI string) bool {
	if originalClientType != domain.ClientTypeOpenAI {
		return false
	}
	path := strings.TrimRight(requestURI, "/")
	if path != "/v1/chat/completions" && !strings.HasSuffix(path, "/v1/chat/completions") {
		return false
	}
	return e.openAIChatStreamTimeoutsEnabled()
}
