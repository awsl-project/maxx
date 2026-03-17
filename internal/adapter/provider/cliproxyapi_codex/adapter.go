package cliproxyapi_codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/exec"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/sjson"
)

// TokenCache caches access tokens
type TokenCache struct {
	AccessToken string
	ExpiresAt   time.Time
}

type CLIProxyAPICodexAdapter struct {
	provider       *domain.Provider
	authObj        *auth.Auth
	executor       *exec.CodexExecutor
	tokenCache     *TokenCache
	tokenMu        sync.RWMutex
	providerUpdate func(*domain.Provider) error
}

// SetProviderUpdateFunc sets the callback for persisting provider updates
func (a *CLIProxyAPICodexAdapter) SetProviderUpdateFunc(fn func(*domain.Provider) error) {
	a.providerUpdate = fn
}

// codexConfig returns the Codex config from the provider.
// CPA adapter always uses ProviderConfigCodex (the real provider's config).
func (a *CLIProxyAPICodexAdapter) codexConfig() *domain.ProviderConfigCodex {
	return a.provider.Config.Codex
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Codex == nil {
		return nil, fmt.Errorf("provider %s missing codex config", p.Name)
	}

	cfg := p.Config.Codex

	// 创建 Auth 对象
	metadata := map[string]any{
		"type":          "codex",
		"refresh_token": cfg.RefreshToken,
	}
	if cfg.AccountID != "" {
		metadata["account_id"] = cfg.AccountID
	}

	authObj := &auth.Auth{
		Provider: "codex",
		Metadata: metadata,
	}

	adapter := &CLIProxyAPICodexAdapter{
		provider:   p,
		authObj:    authObj,
		executor:   exec.NewCodexExecutor(),
		tokenCache: &TokenCache{},
	}

	// 从配置初始化 token 缓存
	if cfg.AccessToken != "" && cfg.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, cfg.ExpiresAt)
		if err == nil && time.Now().Before(expiresAt) {
			adapter.tokenCache = &TokenCache{
				AccessToken: cfg.AccessToken,
				ExpiresAt:   expiresAt,
			}
		}
	}

	return adapter, nil
}

func (a *CLIProxyAPICodexAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeCodex}
}

// WarmToken pre-warms the access token cache to avoid blocking during Execute
func (a *CLIProxyAPICodexAdapter) WarmToken(ctx context.Context) error {
	_, err := a.getAccessToken(ctx)
	return err
}

// getAccessToken 获取有效的 access_token，三级策略：
// 1. 内存缓存
// 2. 配置中的持久化 token
// 3. refresh_token 刷新
func (a *CLIProxyAPICodexAdapter) getAccessToken(ctx context.Context) (string, error) {
	// 检查缓存
	a.tokenMu.RLock()
	if a.tokenCache.AccessToken != "" {
		if a.tokenCache.ExpiresAt.IsZero() || time.Now().Add(60*time.Second).Before(a.tokenCache.ExpiresAt) {
			token := a.tokenCache.AccessToken
			a.tokenMu.RUnlock()
			return token, nil
		}
	}
	a.tokenMu.RUnlock()

	// 使用配置中的 access_token
	cfg := a.codexConfig()
	a.tokenMu.RLock()
	cfgAccessToken := strings.TrimSpace(cfg.AccessToken)
	cfgExpiresAt := strings.TrimSpace(cfg.ExpiresAt)
	cfgRefreshToken := cfg.RefreshToken
	a.tokenMu.RUnlock()

	if cfgAccessToken != "" {
		var expiresAt time.Time
		if cfgExpiresAt != "" {
			if parsed, err := time.Parse(time.RFC3339, cfgExpiresAt); err == nil {
				expiresAt = parsed
			}
		}
		a.tokenMu.Lock()
		a.tokenCache = &TokenCache{
			AccessToken: cfgAccessToken,
			ExpiresAt:   expiresAt,
		}
		a.tokenMu.Unlock()

		if expiresAt.IsZero() || time.Now().Add(60*time.Second).Before(expiresAt) {
			return cfgAccessToken, nil
		}
	}

	// 刷新 token
	tokenResp, err := refreshAccessToken(ctx, cfgRefreshToken)
	if err != nil {
		// 刷新失败时，如果有旧 token 就兜底使用
		if cfgAccessToken != "" {
			return cfgAccessToken, nil
		}
		return "", err
	}

	// 计算过期时间（预留 60s 缓冲，至少保留 1s 避免负值导致无限刷新）
	ttl := tokenResp.ExpiresIn - 60
	if ttl < 1 {
		ttl = 1
	}
	expiresAt := time.Now().Add(time.Duration(ttl) * time.Second)

	// 更新缓存和 cfg 字段在同一个临界区
	a.tokenMu.Lock()
	a.tokenCache = &TokenCache{
		AccessToken: tokenResp.AccessToken,
		ExpiresAt:   expiresAt,
	}
	if a.providerUpdate != nil {
		cfg.AccessToken = tokenResp.AccessToken
		cfg.ExpiresAt = expiresAt.Format(time.RFC3339)
		if tokenResp.RefreshToken != "" {
			cfg.RefreshToken = tokenResp.RefreshToken
		}
	}
	a.tokenMu.Unlock()

	// 持久化 token 到数据库（best-effort，失败不影响当前请求）
	if a.providerUpdate != nil {
		if err := a.providerUpdate(a.provider); err != nil {
			log.Printf("[CLIProxyAPI-Codex] failed to persist refreshed token: %v", err)
		}
	}

	return tokenResp.AccessToken, nil
}

// updateAuthToken 将获取到的 access_token 设置到 authObj.Metadata 中，
// 使 CPA SDK 内部的 codexCreds 能正确读取到 token
func (a *CLIProxyAPICodexAdapter) updateAuthToken(ctx context.Context) error {
	token, err := a.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}
	a.tokenMu.Lock()
	if a.authObj.Metadata == nil {
		a.authObj.Metadata = make(map[string]any)
	}
	a.authObj.Metadata["access_token"] = token
	if !a.tokenCache.ExpiresAt.IsZero() {
		a.authObj.Metadata["expired"] = a.tokenCache.ExpiresAt.Format(time.RFC3339)
	}
	a.tokenMu.Unlock()
	return nil
}

func (a *CLIProxyAPICodexAdapter) Execute(c *flow.Ctx, p *domain.Provider) error {
	w := c.Writer

	requestBody := flow.GetRequestBody(c)
	stream := flow.GetIsStream(c)
	model := flow.GetMappedModel(c)

	// Apply provider-level overrides for reasoning and service_tier
	cfg := a.codexConfig()
	if cfg.Reasoning != "" {
		if updated, err := sjson.SetBytes(requestBody, "reasoning.effort", cfg.Reasoning); err == nil {
			requestBody = updated
		}
	}
	if cfg.ServiceTier != "" {
		if updated, err := sjson.SetBytes(requestBody, "service_tier", cfg.ServiceTier); err == nil {
			requestBody = updated
		}
	}

	// Codex CLI 请求体本质是 OpenAI Responses schema；保持与 CLIProxyAPI 一致。
	sourceFormat := translator.FormatOpenAIResponse

	// 发送事件
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method: "POST",
			URL:    fmt.Sprintf("cliproxyapi://codex/%s", model),
			Body:   string(requestBody),
		})
	}

	// 确保 authObj 中有有效的 access_token
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	if err := a.updateAuthToken(ctx); err != nil {
		return domain.NewProxyErrorWithMessage(err, true, fmt.Sprintf("failed to get access token: %v", err))
	}

	// 构建 executor 请求
	execReq := executor.Request{
		Model:   model,
		Payload: requestBody,
		Format:  sourceFormat,
	}

	execOpts := executor.Options{
		Stream:          stream,
		OriginalRequest: requestBody,
		SourceFormat:    sourceFormat,
	}

	if stream {
		return a.executeStream(c, w, execReq, execOpts)
	}
	return a.executeNonStream(c, w, execReq, execOpts)
}

func (a *CLIProxyAPICodexAdapter) executeNonStream(c *flow.Ctx, w http.ResponseWriter, execReq executor.Request, execOpts executor.Options) error {
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	// CPA 非流式接口会缓冲完整响应后一次性返回，无法暴露真实上游首字节。
	// 这类路径退回到 TotalTimeout 控制，避免把大响应误判成 first-byte timeout。
	flow.DisableFirstByteTimeout(c)

	resp, err := a.executor.Execute(ctx, a.authObj, execReq, execOpts)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, fmt.Sprintf("executor request failed: %v", err))
	}

	if eventChan := flow.GetEventChan(c); eventChan != nil {
		// Send response info
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status: http.StatusOK,
			Body:   string(resp.Payload),
		})

		// Extract and send token usage metrics
		if metrics := usage.ExtractFromResponse(string(resp.Payload)); metrics != nil {
			// Adjust for Codex: input_tokens includes cached_tokens
			metrics = usage.AdjustForClientType(metrics, domain.ClientTypeCodex)
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:  metrics.InputTokens,
				OutputTokens: metrics.OutputTokens,
			})
		}

		// Extract and send response model
		if model := extractModelFromResponse(resp.Payload); model != "" {
			eventChan.SendResponseModel(model)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Payload)

	return nil
}

func (a *CLIProxyAPICodexAdapter) executeStream(c *flow.Ctx, w http.ResponseWriter, execReq executor.Request, execOpts executor.Options) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return a.executeNonStream(c, w, execReq, execOpts)
	}

	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}

	stream, err := a.executor.ExecuteStream(ctx, a.authObj, execReq, execOpts)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, fmt.Sprintf("executor stream request failed: %v", err))
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	eventChan := flow.GetEventChan(c)

	// Collect SSE content for token extraction
	var sseBuffer bytes.Buffer
	var streamErr error
	firstChunkSent := false

	for chunk := range stream.Chunks {
		if chunk.Err != nil {
			log.Printf("[CLIProxyAPI-Codex] stream chunk error: %v", chunk.Err)
			streamErr = chunk.Err
			break
		}
		// Write every chunk including empty lines (SSE event separators)
		sseBuffer.Write(chunk.Payload)
		sseBuffer.WriteByte('\n')
		_, _ = w.Write(chunk.Payload)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()

		// Report TTFT on first non-empty chunk
		if !firstChunkSent && len(chunk.Payload) > 0 && eventChan != nil {
			eventChan.SendFirstToken(time.Now().UnixMilli())
			firstChunkSent = true
		}
	}

	// Send final events
	if eventChan != nil && sseBuffer.Len() > 0 {
		// Send response info
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status: http.StatusOK,
			Body:   sseBuffer.String(),
		})

		// Extract and send token usage metrics
		if metrics := usage.ExtractFromStreamContent(sseBuffer.String()); metrics != nil {
			// Adjust for Codex: input_tokens includes cached_tokens
			metrics = usage.AdjustForClientType(metrics, domain.ClientTypeCodex)
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:  metrics.InputTokens,
				OutputTokens: metrics.OutputTokens,
			})
		}

		// Extract and send response model
		if model := extractModelFromSSE(sseBuffer.String()); model != "" {
			eventChan.SendResponseModel(model)
		}
	}

	// If error occurred before any data was sent, return error to caller
	if streamErr != nil && sseBuffer.Len() == 0 {
		return domain.NewProxyErrorWithMessage(streamErr, true, fmt.Sprintf("stream chunk error: %v", streamErr))
	}

	return nil
}

// extractModelFromResponse extracts the model field from a JSON response body.
func extractModelFromResponse(body []byte) string {
	var resp struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Model != "" {
		return resp.Model
	}
	return ""
}

// extractModelFromSSE extracts the last model field from accumulated SSE content.
func extractModelFromSSE(sseContent string) string {
	var lastModel string
	for line := range strings.SplitSeq(sseContent, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		var chunk struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err == nil && chunk.Model != "" {
			lastModel = chunk.Model
		}
	}
	return lastModel
}

// tokenResponse represents the OAuth token response
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token,omitempty"`
}

const (
	openAITokenURL = "https://auth.openai.com/oauth/token"
	oauthClientID  = "app_EMoamEEZ73f0CkXaXp7hrann"
)

// refreshAccessToken refreshes the access token using a refresh token
func refreshAccessToken(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", oauthClientID)
	data.Set("refresh_token", refreshToken)
	data.Set("scope", "openid profile email")

	req, err := http.NewRequestWithContext(ctx, "POST", openAITokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}
