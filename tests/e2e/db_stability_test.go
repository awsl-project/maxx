package e2e_test

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDBStability_BurstConcurrentRequests(t *testing.T) {
	env := NewTestEnv(t)

	// Create some test data first
	provider := map[string]any{
		"name":    "stability-test-provider",
		"type":    "openai",
		"baseURL": "https://api.example.com",
		"apiKey":  "sk-test",
		"models":  []string{"gpt-4"},
	}
	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Burst of concurrent reads
	const numRequests = 50
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var failCount atomic.Int32

	wg.Add(numRequests)
	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			r := env.AdminGet("/api/admin/providers")
			if r.StatusCode == http.StatusOK {
				successCount.Add(1)
			} else {
				failCount.Add(1)
			}
			r.Body.Close()
		}()
	}
	wg.Wait()

	if failCount.Load() > 0 {
		t.Fatalf("Expected all %d requests to succeed, but %d failed", numRequests, failCount.Load())
	}
	if successCount.Load() != numRequests {
		t.Fatalf("Expected %d successful requests, got %d", numRequests, successCount.Load())
	}
}

func TestDBStability_HealthCheckUnderLoad(t *testing.T) {
	env := NewTestEnv(t)

	// Start background load using direct HTTP requests (not t.Fatal-based helpers)
	const numWorkers = 5
	const requestsPerWorker = 10
	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				select {
				case <-stop:
					return
				default:
				}
				body := fmt.Sprintf(`{"name":"load-%d-%d","type":"openai","baseURL":"https://api.example.com","apiKey":"sk-test","models":["gpt-4"]}`, idx, j)
				req, err := http.NewRequest(http.MethodPost, env.URL("/api/admin/providers"), bytes.NewBufferString(body))
				if err != nil {
					continue
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+env.Token)
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					continue // ignore connection errors in load goroutines
				}
				resp.Body.Close()
			}
		}(i)
	}

	// While load is running, verify health check always responds
	const numHealthChecks = 10
	var healthSuccess atomic.Int32

	for i := 0; i < numHealthChecks; i++ {
		resp, err := http.Get(env.URL("/health"))
		if err == nil && resp.StatusCode == http.StatusOK {
			healthSuccess.Add(1)
			resp.Body.Close()
		} else if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}

	close(stop)
	wg.Wait()

	if healthSuccess.Load() != numHealthChecks {
		t.Fatalf("Health check failed under load: %d/%d succeeded", healthSuccess.Load(), numHealthChecks)
	}
}

func TestDBStability_ConcurrentReadsAndWrites(t *testing.T) {
	env := NewTestEnv(t)

	const numWriters = 10
	const numReaders = 20
	var wg sync.WaitGroup
	var writeSuccess atomic.Int32
	var readSuccess atomic.Int32

	// Writers
	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(idx int) {
			defer wg.Done()
			provider := map[string]any{
				"name":    fmt.Sprintf("rw-provider-%d", idx),
				"type":    "openai",
				"baseURL": fmt.Sprintf("https://api%d.example.com", idx),
				"apiKey":  fmt.Sprintf("sk-test-%d", idx),
				"models":  []string{"gpt-4"},
			}
			r := env.AdminPost("/api/admin/providers", provider)
			if r.StatusCode == http.StatusCreated {
				writeSuccess.Add(1)
			}
			r.Body.Close()
		}(i)
	}

	// Readers (concurrent with writers)
	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()
			r := env.AdminGet("/api/admin/providers")
			if r.StatusCode == http.StatusOK {
				readSuccess.Add(1)
			}
			r.Body.Close()
		}()
	}

	wg.Wait()

	if writeSuccess.Load() != numWriters {
		t.Fatalf("Expected all %d writes to succeed, got %d", numWriters, writeSuccess.Load())
	}
	if readSuccess.Load() != numReaders {
		t.Fatalf("Expected all %d reads to succeed, got %d", numReaders, readSuccess.Load())
	}
}
