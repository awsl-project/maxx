package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

const (
	testFieldDefaultPrompt      = "请用一句话回答：你现在可用吗？"
	testFieldDefaultConcurrency = 4
	testFieldMaxConcurrency     = 10
	testFieldDefaultTimeout     = 30 * time.Second
	testFieldMaxModelsPerProv   = 50
)

type TestFieldModelBenchmarkRequest struct {
	ProviderIDs          []uint64 `json:"providerIDs"`
	Prompt               string   `json:"prompt"`
	Concurrency          int      `json:"concurrency"`
	TimeoutMS            int      `json:"timeoutMs"`
	MaxModelsPerProvider int      `json:"maxModelsPerProvider"`
}

type TestFieldModelBenchmarkProviderSummary struct {
	ProviderID   uint64 `json:"providerID"`
	ProviderName string `json:"providerName"`
	ProviderType string `json:"providerType"`
	Available    bool   `json:"available"`
	ModelCount   int    `json:"modelCount"`
	TestedCount  int    `json:"testedCount"`
	Error        string `json:"error,omitempty"`
}

type TestFieldModelBenchmarkResult struct {
	ProviderID   uint64            `json:"providerID"`
	ProviderName string            `json:"providerName"`
	ProviderType string            `json:"providerType"`
	Model        string            `json:"model"`
	Available    bool              `json:"available"`
	DurationMS   int64             `json:"durationMs"`
	StatusCode   int               `json:"statusCode,omitempty"`
	Error        string            `json:"error,omitempty"`
	Response     string            `json:"response,omitempty"`
	StartedAt    string            `json:"startedAt"`
	FinishedAt   string            `json:"finishedAt"`
	Headers      map[string]string `json:"headers,omitempty"`
}

type TestFieldModelBenchmarkResponse struct {
	Prompt      string                                   `json:"prompt"`
	Concurrency int                                      `json:"concurrency"`
	TimeoutMS   int                                      `json:"timeoutMs"`
	StartedAt   string                                   `json:"startedAt"`
	FinishedAt  string                                   `json:"finishedAt"`
	Providers   []TestFieldModelBenchmarkProviderSummary `json:"providers"`
	Results     []TestFieldModelBenchmarkResult          `json:"results"`
}

type testFieldBenchmarkTarget struct {
	provider *domain.Provider
	model    string
	endpoint string
	apiKey   string
}

func (h *AdminHandler) handleTestField(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) < 3 || parts[2] != "model-benchmark" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req TestFieldModelBenchmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	resp, err := h.runTestFieldModelBenchmark(r.Context(), maxxctx.GetTenantID(r.Context()), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AdminHandler) runTestFieldModelBenchmark(ctx context.Context, tenantID uint64, req TestFieldModelBenchmarkRequest) (*TestFieldModelBenchmarkResponse, error) {
	if len(req.ProviderIDs) == 0 {
		return nil, fmt.Errorf("at least one provider is required")
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = testFieldDefaultPrompt
	}
	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = testFieldDefaultConcurrency
	}
	if concurrency > testFieldMaxConcurrency {
		concurrency = testFieldMaxConcurrency
	}
	timeout := testFieldDefaultTimeout
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}
	if timeout > testFieldDefaultTimeout {
		timeout = testFieldDefaultTimeout
	}
	maxModels := req.MaxModelsPerProvider
	if maxModels <= 0 || maxModels > testFieldMaxModelsPerProv {
		maxModels = testFieldMaxModelsPerProv
	}

	started := time.Now()
	providerSummaries := make([]TestFieldModelBenchmarkProviderSummary, 0, len(req.ProviderIDs))
	targets := make([]testFieldBenchmarkTarget, 0)
	for _, providerID := range uniqueUint64s(req.ProviderIDs) {
		provider, err := h.svc.GetProvider(tenantID, providerID)
		if err != nil || provider == nil {
			providerSummaries = append(providerSummaries, TestFieldModelBenchmarkProviderSummary{ProviderID: providerID, Available: false, Error: "provider not found"})
			continue
		}
		summary := TestFieldModelBenchmarkProviderSummary{ProviderID: provider.ID, ProviderName: provider.Name, ProviderType: provider.Type}
		endpoint, apiKey, ok, errText := testFieldOpenAICompatibleEndpoint(provider)
		if !ok {
			summary.Error = errText
			providerSummaries = append(providerSummaries, summary)
			continue
		}
		modelsReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
		modelsResult := h.fetchProviderRuntimeModels(modelsReq, provider)
		if !modelsResult.Available || len(modelsResult.Models) == 0 {
			summary.Error = strings.TrimSpace(modelsResult.Error)
			if summary.Error == "" {
				summary.Error = "runtime model list unavailable"
			}
			providerSummaries = append(providerSummaries, summary)
			continue
		}
		models := modelsResult.Models
		if len(models) > maxModels {
			models = models[:maxModels]
		}
		summary.Available = true
		summary.ModelCount = len(modelsResult.Models)
		summary.TestedCount = len(models)
		providerSummaries = append(providerSummaries, summary)
		for _, model := range models {
			targets = append(targets, testFieldBenchmarkTarget{provider: provider, model: model, endpoint: endpoint, apiKey: apiKey})
		}
	}

	results := runTestFieldBenchmarkTargets(ctx, targets, prompt, concurrency, timeout)
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Available != results[j].Available {
			return results[i].Available
		}
		if results[i].DurationMS != results[j].DurationMS {
			return results[i].DurationMS < results[j].DurationMS
		}
		return results[i].Model < results[j].Model
	})
	finished := time.Now()
	return &TestFieldModelBenchmarkResponse{
		Prompt:      prompt,
		Concurrency: concurrency,
		TimeoutMS:   int(timeout / time.Millisecond),
		StartedAt:   started.Format(time.RFC3339),
		FinishedAt:  finished.Format(time.RFC3339),
		Providers:   providerSummaries,
		Results:     results,
	}, nil
}

func testFieldOpenAICompatibleEndpoint(provider *domain.Provider) (endpoint string, apiKey string, ok bool, errText string) {
	if provider == nil || provider.Config == nil {
		return "", "", false, "provider config unavailable"
	}
	switch strings.ToLower(strings.TrimSpace(provider.Type)) {
	case "custom", "newapi":
		if provider.Config.Custom == nil {
			return "", "", false, "custom provider config unavailable"
		}
		endpoint, err := providerChatCompletionsURL(customRuntimeModelsBaseURL(provider.Config.Custom))
		if err != nil {
			return "", "", false, err.Error()
		}
		return endpoint, provider.Config.Custom.APIKey, true, ""
	case "openrouter":
		if provider.Config.OpenRouter == nil || strings.TrimSpace(provider.Config.OpenRouter.APIKey) == "" {
			return "", "", false, "openrouter api key unavailable"
		}
		return "https://openrouter.ai/api/v1/chat/completions", provider.Config.OpenRouter.APIKey, true, ""
	default:
		return "", "", false, "test field benchmark currently supports OpenAI-compatible providers only"
	}
}

func runTestFieldBenchmarkTargets(ctx context.Context, targets []testFieldBenchmarkTarget, prompt string, concurrency int, timeout time.Duration) []TestFieldModelBenchmarkResult {
	if concurrency <= 0 {
		concurrency = testFieldDefaultConcurrency
	}
	if concurrency > len(targets) && len(targets) > 0 {
		concurrency = len(targets)
	}
	jobs := make(chan testFieldBenchmarkTarget)
	results := make([]TestFieldModelBenchmarkResult, 0, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				callCtx, cancel := context.WithTimeout(ctx, timeout)
				result := callTestFieldOpenAIChatBenchmark(callCtx, target, prompt)
				cancel()
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
			}
		}()
	}
	for _, target := range targets {
		select {
		case jobs <- target:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return results
		}
	}
	close(jobs)
	wg.Wait()
	return results
}

func callTestFieldOpenAIChatBenchmark(ctx context.Context, target testFieldBenchmarkTarget, prompt string) TestFieldModelBenchmarkResult {
	started := time.Now()
	result := TestFieldModelBenchmarkResult{
		ProviderID:   target.provider.ID,
		ProviderName: target.provider.Name,
		ProviderType: target.provider.Type,
		Model:        target.model,
		StartedAt:    started.Format(time.RFC3339),
	}
	payload := map[string]any{
		"model":       target.model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0,
		"max_tokens":  256,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.endpoint, bytes.NewReader(body))
	if err != nil {
		result.Error = "build request failed: " + err.Error()
		result.FinishedAt = time.Now().Format(time.RFC3339)
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(target.apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(target.apiKey))
	}
	resp, err := http.DefaultClient.Do(req)
	finished := time.Now()
	result.FinishedAt = finished.Format(time.RFC3339)
	result.DurationMS = finished.Sub(started).Milliseconds()
	if err != nil {
		result.Error = "request failed: " + err.Error()
		return result
	}
	defer resp.Body.Close()
	result.StatusCode = resp.StatusCode
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("upstream status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		return result
	}
	text, err := extractOpenAIChatText(respBody)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Available = true
	result.Response = text
	return result
}

func extractOpenAIChatText(respBody []byte) (string, error) {
	var payloadResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &payloadResp); err != nil {
		return "", fmt.Errorf("decode response failed: %w", err)
	}
	if len(payloadResp.Choices) == 0 {
		return "", fmt.Errorf("response choices unavailable")
	}
	text := payloadResp.Choices[0].Message.Content
	if text == "" {
		text = payloadResp.Choices[0].Text
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("response content unavailable")
	}
	return text, nil
}

func uniqueUint64s(values []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(values))
	result := make([]uint64, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
