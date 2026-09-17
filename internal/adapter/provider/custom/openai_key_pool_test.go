package custom

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

func resetOpenAIKeyPoolForTest() {
	defaultOpenAIKeyPool.mu.Lock()
	defer defaultOpenAIKeyPool.mu.Unlock()
	defaultOpenAIKeyPool.nextIndex = make(map[string]int)
	defaultOpenAIKeyPool.cooldown = make(map[string]time.Time)
}

func newOpenAIKeyPoolTestAdapter(t *testing.T, baseURL string) providerAdapterForTest {
	t.Helper()
	adapter, err := NewAdapter(&domain.Provider{
		ID:                   77,
		Name:                 "openai-key-pool",
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: baseURL,
			APIKey:  "sk-old-single",
			APIKeys: []string{"sk-one", "sk-two", "sk-three"},
		}},
	})
	if err != nil {
		t.Fatalf("NewAdapter error: %v", err)
	}
	return adapter
}

type providerAdapterForTest interface {
	Execute(*flow.Ctx, *domain.Provider) error
}

func executeOpenAIKeyPoolRequest(t *testing.T, adapter providerAdapterForTest) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://localhost/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer client-token")
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	ctx.Set(flow.KeyRequestURI, "/v1/chat/completions")
	ctx.Set(flow.KeyRequestBody, []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`))
	ctx.Set(flowKeyOpenAIAPIKeyCooldownSeconds, 3600)
	if err := adapter.Execute(ctx, &domain.Provider{}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	return rec
}

func TestCustomAdapterOpenAIKeyPoolRoundRobin(t *testing.T) {
	resetOpenAIKeyPoolForTest()
	gotAuth := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-test","choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	adapter := newOpenAIKeyPoolTestAdapter(t, server.URL)
	for i := 0; i < 4; i++ {
		rec := executeOpenAIKeyPoolRequest(t, adapter)
		if rec.Code != http.StatusOK {
			t.Fatalf("response status = %d, want 200", rec.Code)
		}
	}

	want := []string{"Bearer sk-one", "Bearer sk-two", "Bearer sk-three", "Bearer sk-one"}
	if len(gotAuth) != len(want) {
		t.Fatalf("upstream calls = %d, want %d (%v)", len(gotAuth), len(want), gotAuth)
	}
	for i := range want {
		if gotAuth[i] != want[i] {
			t.Fatalf("auth[%d] = %q, want %q (all=%v)", i, gotAuth[i], want[i], gotAuth)
		}
	}
}

func TestCustomAdapterOpenAIKeyPoolCoolsLimitedKeyAndRetriesNextKey(t *testing.T) {
	resetOpenAIKeyPoolForTest()
	gotAuth := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		gotAuth = append(gotAuth, auth)
		w.Header().Set("Content-Type", "application/json")
		if auth == "Bearer sk-one" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"rate limit exceeded"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"chatcmpl-test","choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	adapter := newOpenAIKeyPoolTestAdapter(t, server.URL)
	rec := executeOpenAIKeyPoolRequest(t, adapter)
	if rec.Code != http.StatusOK {
		t.Fatalf("response status = %d, want 200", rec.Code)
	}
	want := []string{"Bearer sk-one", "Bearer sk-two"}
	if len(gotAuth) != len(want) {
		t.Fatalf("upstream calls = %d, want %d (%v)", len(gotAuth), len(want), gotAuth)
	}
	for i := range want {
		if gotAuth[i] != want[i] {
			t.Fatalf("auth[%d] = %q, want %q (all=%v)", i, gotAuth[i], want[i], gotAuth)
		}
	}

	gotAuth = gotAuth[:0]
	rec = executeOpenAIKeyPoolRequest(t, adapter)
	if rec.Code != http.StatusOK {
		t.Fatalf("second response status = %d, want 200", rec.Code)
	}
	if len(gotAuth) != 1 || gotAuth[0] != "Bearer sk-three" {
		t.Fatalf("limited key was not skipped on next request: %v", gotAuth)
	}
}

func TestCustomAdapterOpenAIKeyPoolExhaustionDoesNotReturnKeyScopedCooldown(t *testing.T) {
	resetOpenAIKeyPoolForTest()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"rate limit exceeded"}}`)
	}))
	defer server.Close()

	adapter := newOpenAIKeyPoolTestAdapter(t, server.URL)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://localhost/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer client-token")
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	ctx.Set(flow.KeyRequestURI, "/v1/chat/completions")
	ctx.Set(flow.KeyRequestBody, []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`))
	ctx.Set(flowKeyOpenAIAPIKeyCooldownSeconds, 3600)
	err := adapter.Execute(ctx, &domain.Provider{})
	if err == nil {
		t.Fatal("Execute error = nil, want key pool exhausted error")
	}
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("error type = %T, want *domain.ProxyError", err)
	}
	if proxyErr.Scope != domain.ScopeRequest {
		t.Fatalf("scope = %s, want request so provider cooldown is not written", proxyErr.Scope)
	}
	if !proxyErr.Retryable {
		t.Fatal("key pool exhausted should remain retryable so routing can continue")
	}
}
