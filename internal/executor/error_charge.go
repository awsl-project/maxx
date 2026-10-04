package executor

import (
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

const (
	defaultUserPanelErrorChargeThreshold = 2
	defaultUserPanelErrorChargeAmountUSD = 10.0
	userPanelAPITokenDescriptionPrefix   = "managed-by=maxx-user-panel;user-id="
)

type errorChargeKey struct {
	tenantID   uint64
	apiTokenID uint64
}

func (e *Executor) applyUserPanelErrorCharge(state *execState, proxyReq *domain.ProxyRequest, attempt *domain.ProxyUpstreamAttempt) {
	if e == nil || state == nil || proxyReq == nil || e.apiTokenRepo == nil || state.apiTokenID == 0 {
		return
	}
	key := errorChargeKey{tenantID: state.tenantID, apiTokenID: state.apiTokenID}
	if proxyReq.Status == "COMPLETED" {
		e.resetUserPanelErrorChargeStreak(key)
		return
	}
	if proxyReq.Status != "FAILED" && proxyReq.Status != "REJECTED" {
		return
	}
	cfg := e.userPanelErrorChargeConfig()
	if !cfg.enabled || len(cfg.codes) == 0 || !cfg.codes[proxyReq.StatusCode] {
		e.resetUserPanelErrorChargeStreak(key)
		return
	}
	if !e.isUserPanelAPIToken(state.tenantID, state.apiTokenID) {
		return
	}

	streak := e.incrementUserPanelErrorChargeStreak(key)
	if streak < cfg.threshold {
		return
	}
	if err := e.apiTokenRepo.DeductQuotaBalanceToZero(state.tenantID, state.apiTokenID, cfg.amountNanoUSD); err != nil {
		log.Printf("[Executor] user-panel error charge deduct failed token=%d status=%d amount=%d: %v", state.apiTokenID, proxyReq.StatusCode, cfg.amountNanoUSD, err)
		return
	}
	proxyReq.Cost += cfg.amountNanoUSD
	if attempt != nil {
		attempt.Cost += cfg.amountNanoUSD
		if e.attemptRepo != nil {
			_ = e.attemptRepo.Update(attempt)
		}
	}
}

func (e *Executor) resetUserPanelErrorChargeStreak(key errorChargeKey) {
	if e == nil || key.tenantID == 0 || key.apiTokenID == 0 {
		return
	}
	e.errorChargeMu.Lock()
	delete(e.errorChargeStreak, key)
	e.errorChargeMu.Unlock()
}

func (e *Executor) incrementUserPanelErrorChargeStreak(key errorChargeKey) int {
	e.errorChargeMu.Lock()
	defer e.errorChargeMu.Unlock()
	if e.errorChargeStreak == nil {
		e.errorChargeStreak = make(map[errorChargeKey]int)
	}
	e.errorChargeStreak[key]++
	return e.errorChargeStreak[key]
}

func (e *Executor) isUserPanelAPIToken(tenantID, apiTokenID uint64) bool {
	token, err := e.apiTokenRepo.GetByID(tenantID, apiTokenID)
	if err != nil || token == nil {
		return false
	}
	return strings.HasPrefix(token.Description, userPanelAPITokenDescriptionPrefix)
}

type userPanelErrorChargeConfig struct {
	enabled       bool
	codes         map[int]bool
	threshold     int
	amountNanoUSD uint64
}

func (e *Executor) userPanelErrorChargeConfig() userPanelErrorChargeConfig {
	cfg := userPanelErrorChargeConfig{
		enabled:       systemsettingcache.GetBoolean(e.settingsRepo, domain.SettingKeyUserPanelErrorChargeEnabled),
		codes:         map[int]bool{},
		threshold:     defaultUserPanelErrorChargeThreshold,
		amountNanoUSD: dollarsToNanoUSD(defaultUserPanelErrorChargeAmountUSD),
	}
	if e.settingsRepo == nil {
		return cfg
	}
	if raw, err := e.settingsRepo.Get(domain.SettingKeyUserPanelErrorChargeCodes); err == nil {
		cfg.codes = parseErrorChargeCodes(raw)
	}
	if raw, err := e.settingsRepo.Get(domain.SettingKeyUserPanelErrorChargeThreshold); err == nil {
		if threshold, parseErr := strconv.Atoi(strings.TrimSpace(raw)); parseErr == nil && threshold > 0 {
			cfg.threshold = threshold
		}
	}
	if raw, err := e.settingsRepo.Get(domain.SettingKeyUserPanelErrorChargeAmount); err == nil {
		if amount, parseErr := strconv.ParseFloat(strings.TrimSpace(raw), 64); parseErr == nil && amount > 0 {
			cfg.amountNanoUSD = dollarsToNanoUSD(amount)
		}
	}
	return cfg
}

func parseErrorChargeCodes(value string) map[int]bool {
	codes := map[int]bool{}
	for _, field := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		code, err := strconv.Atoi(strings.TrimSpace(field))
		if err == nil && code >= 100 && code <= 599 {
			codes[code] = true
		}
	}
	return codes
}

func dollarsToNanoUSD(amount float64) uint64 {
	return uint64(math.Round(amount * 1_000_000_000))
}
