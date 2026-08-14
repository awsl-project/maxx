package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestCallTestFieldOpenAIChatBenchmark(t *testing.T) {
	var receivedModel string
	var receivedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		receivedModel = payload.Model
		if len(payload.Messages) != 1 {
			t.Fatalf("expected one message, got %d", len(payload.Messages))
		}
		receivedPrompt = payload.Messages[0].Content
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	provider := &domain.Provider{ID: 7, Name: "local", Type: "custom"}
	result := callTestFieldOpenAIChatBenchmark(context.Background(), testFieldBenchmarkTarget{
		provider: provider,
		model:    "gpt-test",
		endpoint: server.URL + "/v1/chat/completions",
		apiKey:   "sk-test",
	}, "ping")

	if !result.Available || result.Response != "ok" || result.Error != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if receivedModel != "gpt-test" || receivedPrompt != "ping" {
		t.Fatalf("unexpected request model/prompt: %q %q", receivedModel, receivedPrompt)
	}
}

func TestRunTestFieldBenchmarkTargetsSortsAvailableByLatencyOutsideCaller(t *testing.T) {
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"fast"}}]}`))
	}))
	defer fast.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"slow"}}]}`))
	}))
	defer slow.Close()

	provider := &domain.Provider{ID: 1, Name: "p", Type: "custom"}
	results, cachedCount := runTestFieldBenchmarkTargets(context.Background(), []testFieldBenchmarkTarget{
		{provider: provider, model: "slow", endpoint: slow.URL, apiKey: ""},
		{provider: provider, model: "fast", endpoint: fast.URL, apiKey: ""},
	}, "ping", 2, time.Second, nil)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if cachedCount != 0 {
		t.Fatalf("expected no cached results, got %d", cachedCount)
	}
	if !results[0].Available || !results[1].Available {
		t.Fatalf("expected available results: %+v", results)
	}
}

func TestRunTestFieldBenchmarkTargetsReportsIncrementalCachedResults(t *testing.T) {
	serverCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	provider := &domain.Provider{ID: 99, Name: "cached", Type: "custom"}
	target := testFieldBenchmarkTarget{provider: provider, model: "cached-model", endpoint: server.URL, reuseCachedResults: true}
	first, firstCached := runTestFieldBenchmarkTargets(context.Background(), []testFieldBenchmarkTarget{target}, "ping", 1, time.Second, nil)
	if len(first) != 1 || !first[0].Available || firstCached != 0 || serverCalls != 1 {
		t.Fatalf("unexpected first run results=%+v cached=%d calls=%d", first, firstCached, serverCalls)
	}

	var incremental []TestFieldModelBenchmarkResult
	second, secondCached := runTestFieldBenchmarkTargets(context.Background(), []testFieldBenchmarkTarget{target}, "ping", 1, time.Second, func(result TestFieldModelBenchmarkResult, cached bool) {
		if !cached || !result.Cached {
			t.Fatalf("expected cached incremental result, got cached=%v result=%+v", cached, result)
		}
		incremental = append(incremental, result)
	})
	if len(second) != 1 || secondCached != 1 || serverCalls != 1 || len(incremental) != 1 {
		t.Fatalf("unexpected cached run results=%+v cached=%d calls=%d incremental=%d", second, secondCached, serverCalls, len(incremental))
	}
}

func TestTestFieldOpenAICompatibleEndpointRejectsUnsupportedProvider(t *testing.T) {
	_, _, ok, errText := testFieldOpenAICompatibleEndpoint(&domain.Provider{Type: "claude", Config: &domain.ProviderConfig{}}, "http://maxx.test")
	if ok || errText == "" {
		t.Fatalf("expected unsupported provider rejection, ok=%v err=%q", ok, errText)
	}
}

func TestTestFieldOpenAICompatibleEndpointSupportsGrokProviderProxy(t *testing.T) {
	endpoint, apiKey, ok, errText := testFieldOpenAICompatibleEndpoint(&domain.Provider{
		ID:   42,
		Type: "grok",
		Config: &domain.ProviderConfig{Grok: &domain.ProviderConfigGrok{
			Type:         "xai",
			AuthKind:     "oauth",
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
		}},
	}, "http://maxx.test")
	if !ok || errText != "" {
		t.Fatalf("expected grok provider proxy endpoint, ok=%v err=%q", ok, errText)
	}
	if endpoint != "http://maxx.test/provider/42/v1/chat/completions" || apiKey != "" {
		t.Fatalf("unexpected endpoint/apiKey: %q %q", endpoint, apiKey)
	}
}
