package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestRunProviderModelCheckCustomOpenAICompatible(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want Bearer sk-test", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "test-model" {
			t.Fatalf("model = %#v, want test-model", payload["model"])
		}
		n := int(calls.Add(1)%5) + 1
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + string(rune('0'+n)) + `"}}]}`))
	}))
	defer server.Close()

	provider := &domain.Provider{
		ID:   7,
		Name: "custom-one",
		Type: "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: server.URL,
			APIKey:  "sk-test",
		}},
	}
	baselineDist := make([]float64, 355)
	baselineDist[0] = 1
	resp, err := runProviderModelCheck(context.Background(), provider, ProviderModelCheckRequest{
		ClientType:  domain.ClientTypeOpenAI,
		Model:       "test-model",
		Iterations:  40,
		Concurrency: 3,
		Baselines: []ProviderModelCheckBase{{
			Name:         "baseline-one",
			Model:        "official-one",
			Distribution: baselineDist,
			Stats:        ProviderModelCheckStats{Mode: 1},
		}},
	})
	if err != nil {
		t.Fatalf("runProviderModelCheck: %v", err)
	}
	if resp.ProviderID != 7 || resp.ProviderName != "custom-one" {
		t.Fatalf("provider identity = %d/%q", resp.ProviderID, resp.ProviderName)
	}
	if !resp.Available || !resp.Reliable {
		t.Fatalf("available/reliable = %v/%v", resp.Available, resp.Reliable)
	}
	if resp.ValidCount != 40 || resp.SuccessCount != 40 || resp.ErrorCount != 0 {
		t.Fatalf("counts = valid %d success %d error %d", resp.ValidCount, resp.SuccessCount, resp.ErrorCount)
	}
	if len(resp.Distribution) != 355 {
		t.Fatalf("distribution length = %d", len(resp.Distribution))
	}
	if resp.Stats.Mode == 0 || resp.Stats.Unique == 0 {
		t.Fatalf("stats not populated: %#v", resp.Stats)
	}
	if len(resp.Matches) != 1 || resp.Matches[0].Baseline.Name != "baseline-one" {
		t.Fatalf("matches = %#v", resp.Matches)
	}
}

func TestRunProviderModelCheckCancelsWithoutDeadlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	provider := &domain.Provider{
		ID:   8,
		Name: "broken-custom",
		Type: "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: server.URL,
			APIKey:  "sk-test",
		}},
	}
	resp, err := runProviderModelCheck(context.Background(), provider, ProviderModelCheckRequest{
		ClientType:  domain.ClientTypeOpenAI,
		Model:       "test-model",
		Iterations:  40,
		Concurrency: 4,
		TimeoutMS:   1000,
	})
	if err != nil {
		t.Fatalf("runProviderModelCheck: %v", err)
	}
	if resp.Available || resp.Reliable || resp.SuccessCount != 0 {
		t.Fatalf("unexpected success response: %#v", resp)
	}
	if resp.ErrorCount == 0 || len(resp.Errors) == 0 {
		t.Fatalf("expected upstream errors, got %#v", resp)
	}
}

func TestRunProviderModelCheckRequiresHalfSuccessForAvailability(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if call <= 4 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"7"}}]}`))
			return
		}
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	provider := &domain.Provider{
		ID:   9,
		Name: "mostly-broken-custom",
		Type: "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: server.URL,
			APIKey:  "sk-test",
		}},
	}
	resp, err := runProviderModelCheck(context.Background(), provider, ProviderModelCheckRequest{
		ClientType:  domain.ClientTypeOpenAI,
		Model:       "test-model",
		Iterations:  10,
		Concurrency: 1,
		TimeoutMS:   1000,
	})
	if err != nil {
		t.Fatalf("runProviderModelCheck: %v", err)
	}
	if resp.Available || resp.SuccessCount != 4 || resp.ErrorCount != 6 {
		t.Fatalf("availability/counts = %v success %d error %d", resp.Available, resp.SuccessCount, resp.ErrorCount)
	}
}

func TestRunProviderModelCheckRejectsUnsupportedProvider(t *testing.T) {
	_, err := runProviderModelCheck(context.Background(), &domain.Provider{Type: "openrouter", Config: &domain.ProviderConfig{}}, ProviderModelCheckRequest{Model: "x", Iterations: 40})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
