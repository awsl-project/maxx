package executor

import (
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

func (e *Executor) configuredGlobalUserAgentOverride() string {
	if e == nil || e.settingsRepo == nil {
		return ""
	}
	if !systemsettingcache.GetBoolean(e.settingsRepo, domain.SettingKeyGlobalUserAgentOverrideEnabled) {
		return ""
	}
	value, err := e.settingsRepo.Get(domain.SettingKeyGlobalUserAgent)
	if err != nil {
		return ""
	}
	return domain.NormalizeGlobalUserAgent(value)
}

func (e *Executor) applyGlobalUserAgentOverride(c *flow.Ctx, state *execState) {
	ua := e.configuredGlobalUserAgentOverride()
	if ua == "" || state == nil {
		return
	}
	if state.requestHeaders == nil {
		state.requestHeaders = http.Header{}
	}
	state.requestHeaders.Set("User-Agent", ua)
	if c != nil {
		if c.Request != nil {
			c.Request.Header.Set("User-Agent", ua)
		}
		c.Set(flow.KeyRequestHeaders, state.requestHeaders)
		c.Set(flow.KeyGlobalUserAgentOverride, ua)
	}
}
