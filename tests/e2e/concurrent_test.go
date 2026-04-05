package e2e_test

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentCreateProviders(t *testing.T) {
	env := NewTestEnv(t)

	const numProviders = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32

	wg.Add(numProviders)
	for i := 0; i < numProviders; i++ {
		go func(idx int) {
			defer wg.Done()
			provider := map[string]any{
				"name":    fmt.Sprintf("concurrent-provider-%d", idx),
				"type":    "openai",
				"baseURL": fmt.Sprintf("https://api%d.example.com", idx),
				"apiKey":  fmt.Sprintf("sk-test-%d", idx),
				"models":  []string{"gpt-4"},
			}
			resp := env.AdminPost("/api/admin/providers", provider)
			if resp.StatusCode == http.StatusCreated {
				successCount.Add(1)
			}
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	if successCount.Load() != numProviders {
		t.Fatalf("Expected all %d providers created successfully, got %d", numProviders, successCount.Load())
	}

	// Verify all providers exist
	resp := env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)

	var providers []any
	DecodeJSON(t, resp, &providers)

	if len(providers) != numProviders {
		t.Fatalf("Expected %d providers in list, got %d", numProviders, len(providers))
	}
}

func TestConcurrentSettingsUpdate(t *testing.T) {
	env := NewTestEnv(t)

	const numUpdates = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32

	wg.Add(numUpdates)
	for i := 0; i < numUpdates; i++ {
		go func(idx int) {
			defer wg.Done()
			body := map[string]any{
				"value": fmt.Sprintf("value-%d", idx),
			}
			resp := env.AdminPut("/api/admin/settings/test-concurrent-key", body)
			if resp.StatusCode == http.StatusOK {
				successCount.Add(1)
			}
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	// All updates should succeed (last-write-wins)
	if successCount.Load() != numUpdates {
		t.Fatalf("Expected all %d settings updates to succeed, got %d", numUpdates, successCount.Load())
	}

	// Verify the setting exists with some value
	resp := env.AdminGet("/api/admin/settings/test-concurrent-key")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["key"] != "test-concurrent-key" {
		t.Fatalf("Expected key 'test-concurrent-key', got %v", result["key"])
	}
	if result["value"] == nil || result["value"] == "" {
		t.Fatal("Expected non-empty value for concurrently updated setting")
	}
}

func TestConcurrentRegisterSameUsername(t *testing.T) {
	env := NewTestEnv(t)

	const numAttempts = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var conflictCount atomic.Int32

	wg.Add(numAttempts)
	for i := 0; i < numAttempts; i++ {
		go func() {
			defer wg.Done()
			body := map[string]any{
				"username": "concurrent-user",
				"password": "Test123!",
			}
			resp := env.RequestWithToken(http.MethodPost, "/api/admin/auth/register", body, env.Token)
			switch resp.StatusCode {
			case http.StatusCreated:
				successCount.Add(1)
			case http.StatusConflict:
				conflictCount.Add(1)
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()

	// Exactly one should succeed, the rest should get conflict
	if successCount.Load() != 1 {
		t.Fatalf("Expected exactly 1 successful registration, got %d", successCount.Load())
	}
	if conflictCount.Load() != numAttempts-1 {
		t.Fatalf("Expected %d conflict responses, got %d", numAttempts-1, conflictCount.Load())
	}
}
