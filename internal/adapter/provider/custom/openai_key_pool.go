package custom

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

const flowKeyOpenAIAPIKeyCooldownSeconds = "openai_api_key_cooldown_seconds"

const defaultOpenAIAPIKeyCooldownSeconds = 3600

type openAIKeyPoolState struct {
	mu        sync.Mutex
	nextIndex map[string]int
	cooldown  map[string]time.Time
}

var defaultOpenAIKeyPool = &openAIKeyPoolState{
	nextIndex: make(map[string]int),
	cooldown:  make(map[string]time.Time),
}

func customOpenAIAPIKeys(cfg *domain.ProviderConfigCustom) []string {
	if cfg == nil {
		return nil
	}
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(cfg.APIKeys)+1)
	for _, raw := range cfg.APIKeys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	// Backward compatibility: if apiKeys is not configured, use the legacy single key.
	// When apiKeys exists, it is the source of truth so a hidden/write-only apiKey
	// placeholder cannot silently re-enter the pool.
	if len(keys) == 0 {
		if key := strings.TrimSpace(cfg.APIKey); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func openAIKeyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}

func openAIKeyPoolRouteKey(providerID uint64, clientType domain.ClientType) string {
	return string(clientType) + ":" + strconvFormatUint(providerID)
}

func strconvFormatUint(v uint64) string {
	// tiny helper avoids importing strconv in files that already import a lot; this file owns it.
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func (s *openAIKeyPoolState) next(providerID uint64, clientType domain.ClientType, keys []string, tried map[string]bool) (string, string, bool) {
	if len(keys) == 0 {
		return "", "", false
	}
	now := time.Now()
	routeKey := openAIKeyPoolRouteKey(providerID, clientType)

	s.mu.Lock()
	defer s.mu.Unlock()

	start := s.nextIndex[routeKey] % len(keys)
	for i := 0; i < len(keys); i++ {
		idx := (start + i) % len(keys)
		key := keys[idx]
		hash := openAIKeyHash(key)
		if tried != nil && tried[hash] {
			continue
		}
		cooldownKey := routeKey + ":" + hash
		if until, ok := s.cooldown[cooldownKey]; ok {
			if now.Before(until) {
				continue
			}
			delete(s.cooldown, cooldownKey)
		}
		s.nextIndex[routeKey] = (idx + 1) % len(keys)
		return key, hash, true
	}
	return "", "", false
}

func (s *openAIKeyPoolState) cooldownKey(providerID uint64, clientType domain.ClientType, keyHash string, until time.Time) {
	if keyHash == "" || until.IsZero() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldown[openAIKeyPoolRouteKey(providerID, clientType)+":"+keyHash] = until
}

func openAIKeyCooldownUntil(c *flow.Ctx, proxyErr *domain.ProxyError) time.Time {
	if proxyErr != nil {
		if proxyErr.CooldownUntil != nil && !proxyErr.CooldownUntil.IsZero() {
			return *proxyErr.CooldownUntil
		}
		if proxyErr.RetryAfter > 0 {
			return time.Now().Add(proxyErr.RetryAfter)
		}
	}
	seconds := defaultOpenAIAPIKeyCooldownSeconds
	if c != nil {
		if raw, ok := c.Get(flowKeyOpenAIAPIKeyCooldownSeconds); ok {
			if v, ok := raw.(int); ok && v >= 1 && v <= 604800 {
				seconds = v
			}
		}
	}
	return time.Now().Add(time.Duration(seconds) * time.Second)
}

func shouldCooldownOpenAIKey(proxyErr *domain.ProxyError) bool {
	if proxyErr == nil || proxyErr.Scope != domain.ScopeKey {
		return false
	}
	switch proxyErr.Reason {
	case domain.CooldownReasonRateLimitExceeded,
		domain.CooldownReasonQuotaExhausted,
		domain.CooldownReasonAuthFailure:
		return true
	default:
		return false
	}
}

func applyOpenAIKeyToRequest(req *http.Request, key string) {
	if req == nil || strings.TrimSpace(key) == "" {
		return
	}
	for _, h := range []string{"Authorization", "Proxy-Authorization", "x-api-key", "x-goog-api-key"} {
		req.Header.Del(h)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
}

func newOpenAIKeyPoolExhaustedError() *domain.ProxyError {
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrNoAvailableProviders, true, "all OpenAI API keys for provider are cooling down or unavailable")
	// Each key already carries its own cooldown. Do not write a provider-level
	// cooldown; let the executor move on to the next route/provider for this
	// request, and let this provider become selectable as keys expire.
	proxyErr.Scope = domain.ScopeRequest
	proxyErr.Reason = domain.CooldownReasonRateLimitExceeded
	proxyErr.HTTPStatusCode = http.StatusTooManyRequests
	return proxyErr
}
