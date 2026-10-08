package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/adapter/client"
	"github.com/awsl-project/maxx/internal/domain"
)

func TestProxySessionIdentityOwnership(t *testing.T) {
	h := &ProxyHandler{clientAdapter: client.NewAdapter()}
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-Session-Id", "shared-client-session")
	resolve := func(tenantID, tokenID uint64, clientType domain.ClientType) string {
		return h.resolveProxySessionID(req, nil, clientType, tenantID, tokenID)
	}
	anonymous := resolve(1, 0, domain.ClientTypeOpenAI)
	if got := resolve(1, 0, domain.ClientTypeOpenAI); got != anonymous {
		t.Fatal("an explicit anonymous session must retain its identity")
	}
	seen := map[string]bool{anonymous: true}
	for _, id := range []string{
		resolve(1, 7, domain.ClientTypeOpenAI),
		resolve(1, 8, domain.ClientTypeOpenAI),
		resolve(2, 0, domain.ClientTypeOpenAI),
		resolve(1, 0, domain.ClientTypeClaude),
	} {
		if seen[id] {
			t.Fatal("different owners or client types shared a session")
		}
		seen[id] = true
	}
	// Echoing an opaque storage ID cannot cross into another owner's session.
	req.Header.Set("X-Session-Id", anonymous)
	if seen[resolve(1, 7, domain.ClientTypeOpenAI)] {
		t.Fatal("client-supplied storage ID selected an existing session")
	}
}

func TestProxyAnonymousSessionIgnoresPlaceholderCredentials(t *testing.T) {
	h := &ProxyHandler{clientAdapter: client.NewAdapter()}
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer arbitrary-placeholder")
	req.Header.Set("x-api-key", "arbitrary-placeholder")
	req.Header.Set("User-Agent", "same-sdk")
	body := []byte(`{"metadata":{"user_id":"same-user"}}`)
	first := h.resolveProxySessionID(req, body, domain.ClientTypeOpenAI, domain.DefaultTenantID, 0)
	second := h.resolveProxySessionID(req, body, domain.ClientTypeOpenAI, domain.DefaultTenantID, 0)
	if first == second {
		t.Fatal("placeholder credentials and a user identity must not create a reusable session")
	}
}
