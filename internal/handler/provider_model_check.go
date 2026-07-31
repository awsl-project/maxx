package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

const (
	modelCheckPrompt          = "请从1到355之间随机选择一个数字，只输出这个数字，不要有任何其他内容。"
	modelCheckDefaultCount    = 50
	modelCheckDefaultConc     = 4
	modelCheckMaxCount        = 500
	modelCheckMaxConcurrency  = 10
	modelCheckDefaultTimeout  = 30 * time.Second
	modelCheckMinValidSamples = 40
)

var modelCheckNumberRE = regexp.MustCompile(`\d+`)

type ProviderModelCheckRequest struct {
	ClientType  domain.ClientType        `json:"clientType"`
	Model       string                   `json:"model"`
	Iterations  int                      `json:"iterations"`
	Concurrency int                      `json:"concurrency"`
	TimeoutMS   int                      `json:"timeoutMs"`
	Baselines   []ProviderModelCheckBase `json:"baselines,omitempty"`
}

type ProviderModelCheckBase struct {
	Name         string                  `json:"name"`
	Model        string                  `json:"model"`
	Distribution []float64               `json:"distribution"`
	Stats        ProviderModelCheckStats `json:"stats"`
	Iterations   int                     `json:"iterations,omitempty"`
}

type ProviderModelCheckStats struct {
	Mean      float64 `json:"mean"`
	Median    float64 `json:"median"`
	StdDev    float64 `json:"stdDev"`
	Min       int     `json:"min"`
	Max       int     `json:"max"`
	Unique    int     `json:"unique"`
	Mode      int     `json:"mode"`
	ModeCount int     `json:"modeCount"`
}

type ProviderModelCheckMatch struct {
	Baseline         ProviderModelCheckBase `json:"baseline"`
	CosineSimilarity float64                `json:"cosineSimilarity"`
	JSDivergence     float64                `json:"jsDivergence"`
	ModeScore        float64                `json:"modeScore"`
	OverallScore     float64                `json:"overallScore"`
	ModeMatch        bool                   `json:"modeMatch"`
}

type ProviderModelCheckResponse struct {
	ProviderID   uint64                    `json:"providerID"`
	ProviderName string                    `json:"providerName"`
	ClientType   domain.ClientType         `json:"clientType"`
	Model        string                    `json:"model"`
	Iterations   int                       `json:"iterations"`
	Concurrency  int                       `json:"concurrency"`
	SuccessCount int                       `json:"successCount"`
	ErrorCount   int                       `json:"errorCount"`
	ValidCount   int                       `json:"validCount"`
	Available    bool                      `json:"available"`
	Reliable     bool                      `json:"reliable"`
	Distribution []float64                 `json:"distribution"`
	Stats        ProviderModelCheckStats   `json:"stats"`
	Matches      []ProviderModelCheckMatch `json:"matches,omitempty"`
	Errors       []string                  `json:"errors,omitempty"`
	DurationMS   int64                     `json:"durationMs"`
}

func (h *AdminHandler) handleProviderModelCheck(w http.ResponseWriter, r *http.Request, id uint64) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req ProviderModelCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	provider, err := h.svc.GetProvider(maxxctx.GetTenantID(r.Context()), id)
	if err != nil || provider == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}

	resp, err := runProviderModelCheck(r.Context(), provider, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func runProviderModelCheck(ctx context.Context, provider *domain.Provider, req ProviderModelCheckRequest) (*ProviderModelCheckResponse, error) {
	if provider == nil || provider.Config == nil || provider.Config.Custom == nil {
		return nil, fmt.Errorf("custom provider config unavailable")
	}
	if provider.Type != "custom" && provider.Type != "newapi" {
		return nil, fmt.Errorf("model check currently supports custom/newapi providers only")
	}
	if req.ClientType == "" {
		req.ClientType = domain.ClientTypeOpenAI
	}
	if req.ClientType != domain.ClientTypeOpenAI {
		return nil, fmt.Errorf("model check currently supports OpenAI-compatible chat completions only")
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if req.Iterations <= 0 {
		req.Iterations = modelCheckDefaultCount
	}
	if req.Iterations < modelCheckMinValidSamples || req.Iterations > modelCheckMaxCount {
		return nil, fmt.Errorf("iterations must be between %d and %d", modelCheckMinValidSamples, modelCheckMaxCount)
	}
	if req.Concurrency <= 0 {
		req.Concurrency = modelCheckDefaultConc
	}
	if req.Concurrency > modelCheckMaxConcurrency {
		req.Concurrency = modelCheckMaxConcurrency
	}
	timeout := modelCheckDefaultTimeout
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}
	if timeout > modelCheckDefaultTimeout {
		timeout = modelCheckDefaultTimeout
	}
	chatURL, err := providerChatCompletionsURL(customRuntimeModelsBaseURL(provider.Config.Custom))
	if err != nil {
		return nil, err
	}

	started := time.Now()
	checkCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var mu sync.Mutex
	results := make([]int, 0, req.Iterations)
	errorsSeen := make([]string, 0, 8)
	successCount := 0
	errorCount := 0
	jobs := make(chan struct{})
	var wg sync.WaitGroup
	workerCount := req.Concurrency
	if workerCount > req.Iterations {
		workerCount = req.Iterations
	}
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				if checkCtx.Err() != nil {
					return
				}
				callCtx, callCancel := context.WithTimeout(checkCtx, timeout)
				num, err := callOpenAIModelCheck(callCtx, chatURL, provider.Config.Custom.APIKey, req.Model)
				callCancel()

				mu.Lock()
				if err != nil {
					errorCount++
					if len(errorsSeen) < 8 {
						errorsSeen = append(errorsSeen, err.Error())
					}
					if errorCount >= 10 && successCount == 0 {
						cancel()
					}
					mu.Unlock()
					continue
				}
				successCount++
				if num > 0 {
					results = append(results, num)
				} else {
					errorCount++
					if len(errorsSeen) < 8 {
						errorsSeen = append(errorsSeen, "response did not contain a number in 1-355")
					}
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < req.Iterations; i++ {
		select {
		case jobs <- struct{}{}:
		case <-checkCtx.Done():
			i = req.Iterations
		}
	}
	close(jobs)
	wg.Wait()

	distribution := modelCheckDistribution(results)
	stats := modelCheckStats(results)
	matches := modelCheckMatches(distribution, stats, req.Baselines)
	return &ProviderModelCheckResponse{
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		ClientType:   req.ClientType,
		Model:        req.Model,
		Iterations:   req.Iterations,
		Concurrency:  req.Concurrency,
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		ValidCount:   len(results),
		Available:    successCount > 0,
		Reliable:     len(results) >= modelCheckMinValidSamples,
		Distribution: distribution,
		Stats:        stats,
		Matches:      matches,
		Errors:       errorsSeen,
		DurationMS:   time.Since(started).Milliseconds(),
	}, nil
}

func providerChatCompletionsURL(baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("custom provider base url unavailable")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid provider base url")
	}
	if strings.HasSuffix(parsed.Path, "/v1") {
		return baseURL + "/chat/completions", nil
	}
	return baseURL + "/v1/chat/completions", nil
}

func callOpenAIModelCheck(ctx context.Context, endpoint, apiKey, model string) (int, error) {
	payload := map[string]any{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": modelCheckPrompt}},
		"temperature": 1.0,
		"max_tokens":  10,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("upstream status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var payloadResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &payloadResp); err != nil {
		return 0, fmt.Errorf("decode response failed: %w", err)
	}
	text := ""
	if len(payloadResp.Choices) > 0 {
		text = payloadResp.Choices[0].Message.Content
		if text == "" {
			text = payloadResp.Choices[0].Text
		}
	}
	return extractModelCheckNumber(text), nil
}

func extractModelCheckNumber(text string) int {
	match := modelCheckNumberRE.FindString(text)
	if match == "" {
		return 0
	}
	var num int
	_, _ = fmt.Sscanf(match, "%d", &num)
	if num < 1 || num > 355 {
		return 0
	}
	return num
}

func modelCheckDistribution(numbers []int) []float64 {
	dist := make([]float64, 355)
	if len(numbers) == 0 {
		return dist
	}
	for _, num := range numbers {
		if num >= 1 && num <= 355 {
			dist[num-1]++
		}
	}
	for i := range dist {
		dist[i] /= float64(len(numbers))
	}
	return dist
}

func modelCheckStats(numbers []int) ProviderModelCheckStats {
	if len(numbers) == 0 {
		return ProviderModelCheckStats{}
	}
	sortedNums := append([]int(nil), numbers...)
	sort.Ints(sortedNums)
	freq := map[int]int{}
	sum := 0
	mode, modeCount := sortedNums[0], 0
	for _, num := range numbers {
		sum += num
		freq[num]++
		if freq[num] > modeCount {
			mode, modeCount = num, freq[num]
		}
	}
	mean := float64(sum) / float64(len(numbers))
	variance := 0.0
	for _, num := range numbers {
		variance += math.Pow(float64(num)-mean, 2)
	}
	variance /= float64(len(numbers))
	return ProviderModelCheckStats{Mean: mean, Median: float64(sortedNums[len(sortedNums)/2]), StdDev: math.Sqrt(variance), Min: sortedNums[0], Max: sortedNums[len(sortedNums)-1], Unique: len(freq), Mode: mode, ModeCount: modeCount}
}

func modelCheckMatches(dist []float64, stats ProviderModelCheckStats, baselines []ProviderModelCheckBase) []ProviderModelCheckMatch {
	matches := make([]ProviderModelCheckMatch, 0, len(baselines))
	for _, b := range baselines {
		if len(b.Distribution) != 355 || b.Stats.Mode == 0 {
			continue
		}
		cos, js := modelCheckSimilarityParts(dist, b.Distribution)
		modeScore := 0.0
		if stats.Mode == b.Stats.Mode {
			modeScore = 1
		} else {
			modeScore = math.Max(0, 1-float64(absInt(stats.Mode-b.Stats.Mode))/50)
		}
		overall := modeScore*0.5 + cos*math.Exp(-js)*0.5
		matches = append(matches, ProviderModelCheckMatch{Baseline: b, CosineSimilarity: cos, JSDivergence: js, ModeScore: modeScore, OverallScore: overall, ModeMatch: stats.Mode == b.Stats.Mode})
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].OverallScore > matches[j].OverallScore })
	if len(matches) > 5 {
		matches = matches[:5]
	}
	return matches
}

func modelCheckSimilarityParts(a, b []float64) (float64, float64) {
	dot, na, nb, js := 0.0, 0.0, 0.0, 0.0
	const eps = 1e-10
	for i := 0; i < 355 && i < len(a) && i < len(b); i++ {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
		p, q := a[i]+eps, b[i]+eps
		m := (p + q) / 2
		js += (p*math.Log(p/m) + q*math.Log(q/m)) / 2
	}
	cos := 0.0
	if na > 0 && nb > 0 {
		cos = dot / (math.Sqrt(na) * math.Sqrt(nb))
	}
	return cos, js
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
