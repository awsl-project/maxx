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

func (e *Executor) streamFirstEventTimeout() time.Duration {
	return e.streamTimeoutSetting(domain.SettingKeyStreamFirstEventTimeoutMS, defaultStreamFirstEventTimeout)
}

func (e *Executor) streamIdleTimeout() time.Duration {
	return e.streamTimeoutSetting(domain.SettingKeyStreamIdleTimeoutMS, defaultStreamIdleTimeout)
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
