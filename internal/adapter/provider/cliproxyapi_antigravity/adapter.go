package cliproxyapi_antigravity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/exec"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

type CLIProxyAPIAntigravityAdapter struct {
	provider *domain.Provider
	authObj  *auth.Auth
	executor *exec.AntigravityExecutor
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.CLIProxyAPIAntigravity == nil {
		return nil, fmt.Errorf("provider %s missing cliproxyapi-antigravity config", p.Name)
	}

	cfg := p.Config.CLIProxyAPIAntigravity

	// 创建 Auth 对象，executor 内部会自动处理 token 刷新
	authObj := &auth.Auth{
		Provider: "antigravity",
		Metadata: map[string]any{
			"type":          "antigravity",
			"refresh_token": cfg.RefreshToken,
			"project_id":    cfg.ProjectID,
		},
	}

	adapter := &CLIProxyAPIAntigravityAdapter{
		provider: p,
		authObj:  authObj,
		executor: exec.NewAntigravityExecutor(),
	}

	return adapter, nil
}

func (a *CLIProxyAPIAntigravityAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeGemini}
}

func (a *CLIProxyAPIAntigravityAdapter) Execute(c *flow.Ctx, p *domain.Provider) error {
	w := c.Writer

	clientType := flow.GetClientType(c)
	requestBody := flow.GetRequestBody(c)
	stream := flow.GetIsStream(c)
	requestModel := flow.GetRequestModel(c)
	model := flow.GetMappedModel(c) // 全局映射后的模型名（已包含 ProviderType 条件）

	log.Printf("[CLIProxyAPI-Antigravity] requestModel=%s, mappedModel=%s, clientType=%s", requestModel, model, clientType)

	// 替换 body 中的 model 字段为映射后的模型名
	requestBody, err := updateModelInBody(requestBody, model)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, false, fmt.Sprintf("failed to update model in body: %v", err))
	}

	// 发送事件
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method: "POST",
			URL:    fmt.Sprintf("cliproxyapi://antigravity/%s", model),
			Body:   string(requestBody),
		})
	}

	// 确定 source format
	var sourceFormat translator.Format
	switch clientType {
	case domain.ClientTypeClaude:
		sourceFormat = translator.FormatClaude
	case domain.ClientTypeGemini:
		sourceFormat = translator.FormatGemini
	default:
		return domain.NewProxyErrorWithMessage(nil, false, fmt.Sprintf("unsupported client type: %s", clientType))
	}

	// 直接透传原始请求给 executor，executor 内部处理格式转换
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

// updateModelInBody 替换 body 中的 model 字段
func updateModelInBody(body []byte, model string) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	req["model"] = model
	return json.Marshal(req)
}

func (a *CLIProxyAPIAntigravityAdapter) executeNonStream(c *flow.Ctx, w http.ResponseWriter, execReq executor.Request, execOpts executor.Options) error {
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}

	resp, err := a.executor.Execute(ctx, a.authObj, execReq, execOpts)
	if err != nil {
		log.Printf("[CLIProxyAPI-Antigravity] executeNonStream error: model=%s, err=%v", execReq.Model, err)
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
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:          metrics.InputTokens,
				OutputTokens:         metrics.OutputTokens,
				CacheReadCount:       metrics.CacheReadCount,
				CacheCreationCount:   metrics.CacheCreationCount,
				Cache5mCreationCount: metrics.Cache5mCreationCount,
				Cache1hCreationCount: metrics.Cache1hCreationCount,
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

func (a *CLIProxyAPIAntigravityAdapter) executeStream(c *flow.Ctx, w http.ResponseWriter, execReq executor.Request, execOpts executor.Options) error {
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
		log.Printf("[CLIProxyAPI-Antigravity] executeStream error: model=%s, err=%v", execReq.Model, err)
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
			log.Printf("[CLIProxyAPI-Antigravity] stream chunk error: %v", chunk.Err)
			streamErr = chunk.Err
			break
		}
		if len(chunk.Payload) > 0 {
			// Payload from executor already includes SSE delimiters (\n\n)
			sseBuffer.Write(chunk.Payload)
			_, _ = w.Write(chunk.Payload)
			flusher.Flush()

			// Report TTFT on first non-empty chunk
			if !firstChunkSent && eventChan != nil {
				eventChan.SendFirstToken(time.Now().UnixMilli())
				firstChunkSent = true
			}
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
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:          metrics.InputTokens,
				OutputTokens:         metrics.OutputTokens,
				CacheReadCount:       metrics.CacheReadCount,
				CacheCreationCount:   metrics.CacheCreationCount,
				Cache5mCreationCount: metrics.Cache5mCreationCount,
				Cache1hCreationCount: metrics.Cache1hCreationCount,
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
