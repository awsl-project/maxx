package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

const (
	testFieldDefaultPrompt        = "请用一句话回答：你现在可用吗？"
	testFieldDefaultConcurrency   = 4
	testFieldMaxConcurrency       = 10
	testFieldDefaultTimeout       = 30 * time.Second
	testFieldMaxModelsPerProv     = 50
	testFieldModelCacheTTL        = 2 * time.Minute
	testFieldResultCacheTTL       = 5 * time.Minute
	testFieldFinishedJobRetention = 10 * time.Minute
)

type TestFieldModelBenchmarkRequest struct {
	ProviderIDs           []uint64 `json:"providerIDs"`
	Prompt                string   `json:"prompt"`
	Concurrency           int      `json:"concurrency"`
	TimeoutMS             int      `json:"timeoutMs"`
	MinModelsPerProvider  int      `json:"minModelsPerProvider"`
	MaxModelsPerProvider  int      `json:"maxModelsPerProvider"` // Backward-compatible alias for older clients.
	ReuseCachedResults    *bool    `json:"reuseCachedResults,omitempty"`
	ReuseCachedModelLists *bool    `json:"reuseCachedModelLists,omitempty"`
}

type TestFieldModelBenchmarkProviderSummary struct {
	ProviderID   uint64 `json:"providerID"`
	ProviderName string `json:"providerName"`
	ProviderType string `json:"providerType"`
	Available    bool   `json:"available"`
	ModelCount   int    `json:"modelCount"`
	TestedCount  int    `json:"testedCount"`
	Error        string `json:"error,omitempty"`
	CachedModels bool   `json:"cachedModels,omitempty"`
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
	Cached       bool              `json:"cached,omitempty"`
}

type TestFieldModelBenchmarkResponse struct {
	JobID                string                                   `json:"jobID,omitempty"`
	Status               string                                   `json:"status,omitempty"`
	Prompt               string                                   `json:"prompt"`
	Concurrency          int                                      `json:"concurrency"`
	TimeoutMS            int                                      `json:"timeoutMs"`
	MinModelsPerProvider int                                      `json:"minModelsPerProvider"`
	StartedAt            string                                   `json:"startedAt"`
	FinishedAt           string                                   `json:"finishedAt"`
	Providers            []TestFieldModelBenchmarkProviderSummary `json:"providers"`
	Results              []TestFieldModelBenchmarkResult          `json:"results"`
	TotalTargets         int                                      `json:"totalTargets"`
	CompletedTargets     int                                      `json:"completedTargets"`
	CachedResultCount    int                                      `json:"cachedResultCount"`
	Error                string                                   `json:"error,omitempty"`
}

type TestFieldModelBenchmarkJobStartResponse struct {
	JobID string `json:"jobID"`
}

type testFieldBenchmarkTarget struct {
	provider           *domain.Provider
	model              string
	endpoint           string
	apiKey             string
	prompt             string
	timeoutMS          int
	reuseCachedResults bool
}

type testFieldModelListCacheEntry struct {
	models    []string
	available bool
	errorText string
	expiresAt time.Time
}

type testFieldResultCacheEntry struct {
	result    TestFieldModelBenchmarkResult
	expiresAt time.Time
}

type testFieldBenchmarkJob struct {
	id                   string
	tenantID             uint64
	request              TestFieldModelBenchmarkRequest
	status               string
	prompt               string
	concurrency          int
	timeout              time.Duration
	minModelsPerProvider int
	startedAt            time.Time
	finishedAt           time.Time
	providers            []TestFieldModelBenchmarkProviderSummary
	results              []TestFieldModelBenchmarkResult
	totalTargets         int
	completedTargets     int
	cachedResultCount    int
	errorText            string
	cancel               context.CancelFunc
	mu                   sync.Mutex
}

var testFieldBenchmarks = struct {
	sync.Mutex
	jobs       map[string]*testFieldBenchmarkJob
	modelLists map[string]testFieldModelListCacheEntry
	results    map[string]testFieldResultCacheEntry
	seq        uint64
}{
	jobs:       make(map[string]*testFieldBenchmarkJob),
	modelLists: make(map[string]testFieldModelListCacheEntry),
	results:    make(map[string]testFieldResultCacheEntry),
}

func (h *AdminHandler) handleTestField(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) < 3 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	switch parts[2] {
	case "model-benchmark":
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
	case "model-benchmark-jobs":
		h.handleTestFieldModelBenchmarkJobs(w, r, parts)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (h *AdminHandler) handleTestFieldModelBenchmarkJobs(w http.ResponseWriter, r *http.Request, parts []string) {
	tenantID := maxxctx.GetTenantID(r.Context())
	if len(parts) == 3 {
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
		job, err := h.startTestFieldModelBenchmarkJob(tenantID, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, TestFieldModelBenchmarkJobStartResponse{JobID: job.id})
		return
	}
	if len(parts) != 4 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	jobID := strings.TrimSpace(parts[3])
	job := getTestFieldBenchmarkJob(tenantID, jobID)
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, job.snapshot())
	case http.MethodDelete:
		job.cancelJob()
		writeJSON(w, http.StatusOK, job.snapshot())
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *AdminHandler) startTestFieldModelBenchmarkJob(tenantID uint64, req TestFieldModelBenchmarkRequest) (*testFieldBenchmarkJob, error) {
	prompt, concurrency, timeout, minModels, err := normalizeTestFieldBenchmarkRequest(req)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &testFieldBenchmarkJob{
		id:                   nextTestFieldBenchmarkJobID(),
		tenantID:             tenantID,
		request:              req,
		status:               "running",
		prompt:               prompt,
		concurrency:          concurrency,
		timeout:              timeout,
		minModelsPerProvider: minModels,
		startedAt:            time.Now(),
		cancel:               cancel,
	}
	putTestFieldBenchmarkJob(job)
	go h.runTestFieldModelBenchmarkJob(ctx, job)
	return job, nil
}

func (h *AdminHandler) runTestFieldModelBenchmark(ctx context.Context, tenantID uint64, req TestFieldModelBenchmarkRequest) (*TestFieldModelBenchmarkResponse, error) {
	prompt, concurrency, timeout, minModels, err := normalizeTestFieldBenchmarkRequest(req)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	providers, targets := h.buildTestFieldBenchmarkTargets(ctx, tenantID, req, minModels)
	results, cachedCount := runTestFieldBenchmarkTargets(ctx, targets, prompt, concurrency, timeout, nil)
	results = sortTestFieldBenchmarkResults(results)
	finished := time.Now()
	return &TestFieldModelBenchmarkResponse{
		Status:               "completed",
		Prompt:               prompt,
		Concurrency:          concurrency,
		TimeoutMS:            int(timeout / time.Millisecond),
		MinModelsPerProvider: minModels,
		StartedAt:            started.Format(time.RFC3339),
		FinishedAt:           finished.Format(time.RFC3339),
		Providers:            providers,
		Results:              results,
		TotalTargets:         len(targets),
		CompletedTargets:     len(results),
		CachedResultCount:    cachedCount,
	}, nil
}

func normalizeTestFieldBenchmarkRequest(req TestFieldModelBenchmarkRequest) (string, int, time.Duration, int, error) {
	if len(req.ProviderIDs) == 0 {
		return "", 0, 0, 0, fmt.Errorf("at least one provider is required")
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
	minModels := req.MinModelsPerProvider
	if minModels <= 0 {
		minModels = req.MaxModelsPerProvider
	}
	if minModels <= 0 || minModels > testFieldMaxModelsPerProv {
		minModels = testFieldMaxModelsPerProv
	}
	return prompt, concurrency, timeout, minModels, nil
}

func (h *AdminHandler) runTestFieldModelBenchmarkJob(ctx context.Context, job *testFieldBenchmarkJob) {
	providers, targets := h.buildTestFieldBenchmarkTargets(ctx, job.tenantID, job.request, job.minModelsPerProvider)
	job.mu.Lock()
	job.providers = providers
	job.totalTargets = len(targets)
	job.mu.Unlock()

	results, cachedCount := runTestFieldBenchmarkTargets(ctx, targets, job.prompt, job.concurrency, job.timeout, func(result TestFieldModelBenchmarkResult, cached bool) {
		job.mu.Lock()
		job.results = sortTestFieldBenchmarkResults(append(job.results, result))
		job.completedTargets = len(job.results)
		if cached {
			job.cachedResultCount++
		}
		job.mu.Unlock()
	})

	job.mu.Lock()
	defer job.mu.Unlock()
	job.results = sortTestFieldBenchmarkResults(results)
	job.completedTargets = len(results)
	job.cachedResultCount = cachedCount
	job.finishedAt = time.Now()
	if ctx.Err() != nil {
		job.status = "cancelled"
		job.errorText = ctx.Err().Error()
	} else {
		job.status = "completed"
	}
}

func (h *AdminHandler) buildTestFieldBenchmarkTargets(ctx context.Context, tenantID uint64, req TestFieldModelBenchmarkRequest, minModels int) ([]TestFieldModelBenchmarkProviderSummary, []testFieldBenchmarkTarget) {
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
		modelsResult, cachedModels := h.fetchTestFieldRuntimeModels(ctx, provider, reuseBool(req.ReuseCachedModelLists, true))
		if !modelsResult.Available || len(modelsResult.Models) == 0 {
			summary.Error = strings.TrimSpace(modelsResult.Error)
			if summary.Error == "" {
				summary.Error = "runtime model list unavailable"
			}
			providerSummaries = append(providerSummaries, summary)
			continue
		}
		models := modelsResult.Models
		if len(models) > minModels {
			models = models[:minModels]
		}
		summary.Available = true
		summary.ModelCount = len(modelsResult.Models)
		summary.TestedCount = len(models)
		summary.CachedModels = cachedModels
		providerSummaries = append(providerSummaries, summary)
		for _, model := range models {
			targets = append(targets, testFieldBenchmarkTarget{provider: provider, model: model, endpoint: endpoint, apiKey: apiKey, reuseCachedResults: reuseBool(req.ReuseCachedResults, true), prompt: strings.TrimSpace(req.Prompt)})
		}
	}
	return providerSummaries, targets
}

func (h *AdminHandler) fetchTestFieldRuntimeModels(ctx context.Context, provider *domain.Provider, reuseCache bool) (providerRuntimeModelsResult, bool) {
	cacheKey := testFieldProviderCacheKey(provider)
	if reuseCache {
		testFieldBenchmarks.Lock()
		entry, ok := testFieldBenchmarks.modelLists[cacheKey]
		testFieldBenchmarks.Unlock()
		if ok && time.Now().Before(entry.expiresAt) {
			return providerRuntimeModelsResult{Available: entry.available, Models: append([]string(nil), entry.models...), Error: entry.errorText}, true
		}
	}
	modelsReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	modelsResult := h.fetchProviderRuntimeModels(modelsReq, provider)
	testFieldBenchmarks.Lock()
	testFieldBenchmarks.modelLists[cacheKey] = testFieldModelListCacheEntry{
		models:    append([]string(nil), modelsResult.Models...),
		available: modelsResult.Available,
		errorText: strings.TrimSpace(modelsResult.Error),
		expiresAt: time.Now().Add(testFieldModelCacheTTL),
	}
	testFieldBenchmarks.Unlock()
	return modelsResult, false
}

func runTestFieldBenchmarkTargets(ctx context.Context, targets []testFieldBenchmarkTarget, prompt string, concurrency int, timeout time.Duration, onResult func(TestFieldModelBenchmarkResult, bool)) ([]TestFieldModelBenchmarkResult, int) {
	if concurrency <= 0 {
		concurrency = testFieldDefaultConcurrency
	}
	if concurrency > len(targets) && len(targets) > 0 {
		concurrency = len(targets)
	}
	if concurrency <= 0 {
		return nil, 0
	}
	jobs := make(chan testFieldBenchmarkTarget)
	results := make([]TestFieldModelBenchmarkResult, 0, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	cachedCount := 0
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				result, cached := callTestFieldOpenAIChatBenchmarkWithCache(ctx, target, prompt, timeout)
				mu.Lock()
				results = append(results, result)
				if cached {
					cachedCount++
				}
				mu.Unlock()
				if onResult != nil {
					onResult(result, cached)
				}
			}
		}()
	}
	for _, target := range targets {
		select {
		case jobs <- target:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return results, cachedCount
		}
	}
	close(jobs)
	wg.Wait()
	return results, cachedCount
}

func callTestFieldOpenAIChatBenchmarkWithCache(ctx context.Context, target testFieldBenchmarkTarget, prompt string, timeout time.Duration) (TestFieldModelBenchmarkResult, bool) {
	cacheKey := testFieldResultCacheKey(target, prompt, timeout)
	if target.reuseCachedResults {
		testFieldBenchmarks.Lock()
		entry, ok := testFieldBenchmarks.results[cacheKey]
		testFieldBenchmarks.Unlock()
		if ok && time.Now().Before(entry.expiresAt) {
			cached := entry.result
			cached.Cached = true
			return cached, true
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	result := callTestFieldOpenAIChatBenchmark(callCtx, target, prompt)
	cancel()
	testFieldBenchmarks.Lock()
	testFieldBenchmarks.results[cacheKey] = testFieldResultCacheEntry{result: result, expiresAt: time.Now().Add(testFieldResultCacheTTL)}
	testFieldBenchmarks.Unlock()
	return result, false
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

func sortTestFieldBenchmarkResults(results []TestFieldModelBenchmarkResult) []TestFieldModelBenchmarkResult {
	sorted := append([]TestFieldModelBenchmarkResult(nil), results...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Available != sorted[j].Available {
			return sorted[i].Available
		}
		if sorted[i].DurationMS != sorted[j].DurationMS {
			return sorted[i].DurationMS < sorted[j].DurationMS
		}
		return sorted[i].Model < sorted[j].Model
	})
	return sorted
}

func (j *testFieldBenchmarkJob) snapshot() TestFieldModelBenchmarkResponse {
	j.mu.Lock()
	defer j.mu.Unlock()
	finishedAt := ""
	if !j.finishedAt.IsZero() {
		finishedAt = j.finishedAt.Format(time.RFC3339)
	}
	return TestFieldModelBenchmarkResponse{
		JobID:                j.id,
		Status:               j.status,
		Prompt:               j.prompt,
		Concurrency:          j.concurrency,
		TimeoutMS:            int(j.timeout / time.Millisecond),
		MinModelsPerProvider: j.minModelsPerProvider,
		StartedAt:            j.startedAt.Format(time.RFC3339),
		FinishedAt:           finishedAt,
		Providers:            append([]TestFieldModelBenchmarkProviderSummary(nil), j.providers...),
		Results:              append([]TestFieldModelBenchmarkResult(nil), j.results...),
		TotalTargets:         j.totalTargets,
		CompletedTargets:     j.completedTargets,
		CachedResultCount:    j.cachedResultCount,
		Error:                j.errorText,
	}
}

func (j *testFieldBenchmarkJob) cancelJob() {
	j.mu.Lock()
	alreadyDone := j.status == "completed" || j.status == "cancelled" || j.status == "failed"
	j.mu.Unlock()
	if !alreadyDone && j.cancel != nil {
		j.cancel()
	}
}

func nextTestFieldBenchmarkJobID() string {
	testFieldBenchmarks.Lock()
	defer testFieldBenchmarks.Unlock()
	testFieldBenchmarks.seq++
	return fmt.Sprintf("tfb-%d-%d", time.Now().UnixNano(), testFieldBenchmarks.seq)
}

func putTestFieldBenchmarkJob(job *testFieldBenchmarkJob) {
	testFieldBenchmarks.Lock()
	defer testFieldBenchmarks.Unlock()
	cleanupTestFieldBenchmarkJobsLocked(time.Now())
	testFieldBenchmarks.jobs[job.id] = job
}

func getTestFieldBenchmarkJob(tenantID uint64, jobID string) *testFieldBenchmarkJob {
	testFieldBenchmarks.Lock()
	defer testFieldBenchmarks.Unlock()
	cleanupTestFieldBenchmarkJobsLocked(time.Now())
	job := testFieldBenchmarks.jobs[jobID]
	if job == nil || job.tenantID != tenantID {
		return nil
	}
	return job
}

func cleanupTestFieldBenchmarkJobsLocked(now time.Time) {
	for id, job := range testFieldBenchmarks.jobs {
		job.mu.Lock()
		finishedAt := job.finishedAt
		status := job.status
		job.mu.Unlock()
		if (status == "completed" || status == "cancelled" || status == "failed") && !finishedAt.IsZero() && now.Sub(finishedAt) > testFieldFinishedJobRetention {
			delete(testFieldBenchmarks.jobs, id)
		}
	}
	for key, entry := range testFieldBenchmarks.modelLists {
		if now.After(entry.expiresAt) {
			delete(testFieldBenchmarks.modelLists, key)
		}
	}
	for key, entry := range testFieldBenchmarks.results {
		if now.After(entry.expiresAt) {
			delete(testFieldBenchmarks.results, key)
		}
	}
}

func testFieldProviderCacheKey(provider *domain.Provider) string {
	if provider == nil {
		return "nil"
	}
	endpoint, apiKey, _, _ := testFieldOpenAICompatibleEndpoint(provider)
	return hashStrings(strconv.FormatUint(provider.ID, 10), provider.Type, endpoint, apiKey)
}

func testFieldResultCacheKey(target testFieldBenchmarkTarget, prompt string, timeout time.Duration) string {
	return hashStrings(strconv.FormatUint(target.provider.ID, 10), target.provider.Type, target.endpoint, target.apiKey, target.model, prompt, strconv.FormatInt(int64(timeout/time.Millisecond), 10))
}

func hashStrings(values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func reuseBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
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
