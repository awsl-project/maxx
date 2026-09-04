package cliproxyapi_grok

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
	"github.com/awsl-project/maxx/internal/adapter/provider/cliproxyerr"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/exec"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

type CLIProxyAPIGrokAdapter struct {
	provider *domain.Provider
	authObj  *auth.Auth
	executor grokExecutor
}

type grokExecutor interface {
	Execute(context.Context, *auth.Auth, executor.Request, executor.Options) (executor.Response, error)
	ExecuteStream(context.Context, *auth.Auth, executor.Request, executor.Options) (*executor.StreamResult, error)
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p == nil || p.Config == nil || p.Config.Grok == nil {
		return nil, fmt.Errorf("provider missing grok config")
	}
	cfg := p.Config.Grok
	if strings.TrimSpace(cfg.Type) == "" {
		cfg.Type = "xai"
	}
	if strings.TrimSpace(cfg.AuthKind) == "" {
		cfg.AuthKind = "oauth"
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Type), "xai") {
		return nil, fmt.Errorf("grok provider only accepts CPA xai credentials, got type %q", cfg.Type)
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.AuthKind), "oauth") {
		return nil, fmt.Errorf("grok provider only accepts oauth credentials, got auth_kind %q", cfg.AuthKind)
	}
	if strings.TrimSpace(cfg.AccessToken) == "" && strings.TrimSpace(cfg.RefreshToken) == "" {
		return nil, fmt.Errorf("grok provider requires access_token or refresh_token")
	}

	metadata := map[string]any{
		"type":      "xai",
		"auth_kind": "oauth",
	}
	put := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			metadata[key] = value
		}
	}
	put("email", cfg.Email)
	put("sub", cfg.Sub)
	put("access_token", cfg.AccessToken)
	put("refresh_token", cfg.RefreshToken)
	put("id_token", cfg.IDToken)
	put("token_type", cfg.TokenType)
	put("expired", cfg.Expired)
	put("last_refresh", cfg.LastRefresh)
	put("redirect_uri", cfg.RedirectURI)
	put("token_endpoint", cfg.TokenEndpoint)
	put("base_url", cfg.BaseURL)
	if cfg.ExpiresIn > 0 {
		metadata["expires_in"] = cfg.ExpiresIn
	}
	if len(cfg.Headers) > 0 {
		headers := make(map[string]string, len(cfg.Headers))
		for k, v := range cfg.Headers {
			if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
				headers[k] = v
			}
		}
		metadata["headers"] = headers
	}

	attributes := map[string]string{
		"auth_kind": "oauth",
	}
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		attributes["base_url"] = baseURL
	}
	if cfg.Email != "" {
		attributes["email"] = cfg.Email
	}

	authObj := &auth.Auth{
		ID:         fmt.Sprintf("maxx-grok-%d", p.ID),
		Provider:   "xai",
		Attributes: attributes,
		Metadata:   metadata,
		Disabled:   cfg.Disabled,
	}

	return &CLIProxyAPIGrokAdapter{
		provider: p,
		authObj:  authObj,
		executor: exec.NewXAIExecutor(),
	}, nil
}

func (a *CLIProxyAPIGrokAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *CLIProxyAPIGrokAdapter) Execute(c *flow.Ctx, p *domain.Provider) error {
	w := c.Writer
	clientType := flow.GetClientType(c)
	if clientType != domain.ClientTypeOpenAI {
		proxyErr := domain.NewProxyErrorWithMessage(nil, false, fmt.Sprintf("grok provider only supports openai client type, got %s", clientType))
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	requestBody := flow.GetRequestBody(c)
	stream := flow.GetIsStream(c)
	requestModel := flow.GetRequestModel(c)
	model := flow.GetMappedModel(c)
	if strings.TrimSpace(model) == "" {
		model = requestModel
	}

	log.Printf("[CLIProxyAPI-Grok] requestModel=%s, mappedModel=%s, clientType=%s", requestModel, model, clientType)

	var err error
	requestBody, err = updateModelInBody(requestBody, model)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, fmt.Sprintf("failed to update model in body: %v", err))
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method: "POST",
			URL:    fmt.Sprintf("cliproxyapi://xai/%s", model),
			Body:   string(requestBody),
		})
	}

	sourceFormat := translator.FormatOpenAI
	metadata := requestMetadata(flow.GetRequestURI(c))
	if isOpenAIImagesRequest(flow.GetRequestURI(c)) {
		sourceFormat = translator.FromString("openai-image")
	}
	execReq := executor.Request{Model: model, Payload: requestBody, Format: sourceFormat, Metadata: metadata}
	execOpts := executor.Options{Stream: stream, OriginalRequest: requestBody, SourceFormat: sourceFormat, Metadata: metadata}
	if stream {
		return a.executeStream(c, w, execReq, execOpts)
	}
	return a.executeNonStream(c, w, execReq, execOpts)
}

func requestMetadata(requestURI string) map[string]any {
	path := requestPath(requestURI)
	if path == "" {
		return nil
	}
	return map[string]any{executor.RequestPathMetadataKey: path}
}

func requestPath(requestURI string) string {
	path := strings.TrimSpace(requestURI)
	if path == "" {
		return ""
	}
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	return strings.TrimSpace(path)
}

func isOpenAIImagesRequest(requestURI string) bool {
	path := requestPath(requestURI)
	return path == "/v1/images" || path == "/images" ||
		strings.HasPrefix(path, "/v1/images/") || strings.HasPrefix(path, "/images/")
}

func updateModelInBody(body []byte, model string) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	req["model"] = model
	return json.Marshal(req)
}

func (a *CLIProxyAPIGrokAdapter) executeNonStream(c *flow.Ctx, w http.ResponseWriter, execReq executor.Request, execOpts executor.Options) error {
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	resp, err := a.executor.Execute(ctx, a.authObj, execReq, execOpts)
	if err != nil {
		log.Printf("[CLIProxyAPI-Grok] executeNonStream error: model=%s, err=%v", execReq.Model, err)
		return cliproxyerr.Classify(err, execReq.Model, fmt.Sprintf("executor request failed: %v", err),
			domain.ScopeProvider, domain.CooldownReasonServerError)
	}
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendResponseInfo(&domain.ResponseInfo{Status: http.StatusOK, Body: string(resp.Payload)})
		if metrics := usage.ExtractFromResponse(string(resp.Payload)); metrics != nil {
			eventChan.SendMetrics(&domain.AdapterMetrics{InputTokens: metrics.InputTokens, OutputTokens: metrics.OutputTokens, CacheReadCount: metrics.CacheReadCount, CacheCreationCount: metrics.CacheCreationCount, Cache5mCreationCount: metrics.Cache5mCreationCount, Cache1hCreationCount: metrics.Cache1hCreationCount})
		}
		if model := extractModelFromResponse(resp.Payload); model != "" {
			eventChan.SendResponseModel(model)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Payload)
	return nil
}

func (a *CLIProxyAPIGrokAdapter) executeStream(c *flow.Ctx, w http.ResponseWriter, execReq executor.Request, execOpts executor.Options) error {
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
		log.Printf("[CLIProxyAPI-Grok] executeStream error: model=%s, err=%v", execReq.Model, err)
		return cliproxyerr.Classify(err, execReq.Model, fmt.Sprintf("executor stream request failed: %v", err),
			domain.ScopeProvider, domain.CooldownReasonServerError)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	eventChan := flow.GetEventChan(c)
	var sseBuffer bytes.Buffer
	firstChunkSent := false
	sawFinishReason := false
	sawDone := false
	var pendingSSE string
	var streamErr error
	for chunk := range stream.Chunks {
		if chunk.Err != nil {
			streamErr = chunk.Err
			break
		}
		if len(chunk.Payload) > 0 {
			sseBuffer.Write(chunk.Payload)
			out, rest, chunkSawFinishReason, chunkSawDone := ensureOpenAIStreamFinishBeforeDoneWithPending(pendingSSE, chunk.Payload, execReq.Model, sawFinishReason)
			pendingSSE = rest
			if chunkSawFinishReason {
				sawFinishReason = true
			}
			if chunkSawDone {
				sawDone = true
			}
			_, _ = w.Write(out)
			flusher.Flush()
			if !firstChunkSent && eventChan != nil {
				eventChan.SendFirstToken(time.Now().UnixMilli())
				firstChunkSent = true
			}
		}
	}
	if streamErr == nil {
		if pendingSSE != "" {
			out, _, pendingSawFinishReason, pendingSawDone := ensureOpenAIStreamFinishBeforeDoneWithPending("", []byte(pendingSSE), execReq.Model, sawFinishReason)
			if pendingSawFinishReason {
				sawFinishReason = true
			}
			if pendingSawDone {
				sawDone = true
			}
			_, _ = w.Write(out)
		}
		if !sawDone {
			if !sawFinishReason {
				_, _ = w.Write(openAIStreamFinishChunk(execReq.Model))
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
		flusher.Flush()
	}
	if eventChan != nil && sseBuffer.Len() > 0 {
		eventChan.SendResponseInfo(&domain.ResponseInfo{Status: http.StatusOK, Body: sseBuffer.String()})
		if metrics := usage.ExtractFromStreamContent(sseBuffer.String()); metrics != nil {
			eventChan.SendMetrics(&domain.AdapterMetrics{InputTokens: metrics.InputTokens, OutputTokens: metrics.OutputTokens, CacheReadCount: metrics.CacheReadCount, CacheCreationCount: metrics.CacheCreationCount, Cache5mCreationCount: metrics.Cache5mCreationCount, Cache1hCreationCount: metrics.Cache1hCreationCount})
		}
		if model := extractModelFromStream(sseBuffer.String()); model != "" {
			eventChan.SendResponseModel(model)
		}
	}
	if streamErr != nil {
		return cliproxyerr.Classify(streamErr, execReq.Model, fmt.Sprintf("stream error: %v", streamErr),
			domain.ScopeProvider, domain.CooldownReasonServerError)
	}
	return nil
}

func ensureOpenAIStreamFinishBeforeDone(payload []byte, model string, alreadySawFinishReason bool) ([]byte, bool, bool) {
	out, _, sawFinishReason, sawDone := ensureOpenAIStreamFinishBeforeDoneWithPending("", payload, model, alreadySawFinishReason)
	return out, sawFinishReason, sawDone
}

func ensureOpenAIStreamFinishBeforeDoneWithPending(pending string, payload []byte, model string, alreadySawFinishReason bool) ([]byte, string, bool, bool) {
	if pending == "" && len(payload) == 0 {
		return payload, "", false, false
	}
	combined := pending + string(payload)
	var out bytes.Buffer
	sawFinishReason := false
	sawDone := false
	finishInserted := false
	for len(combined) > 0 {
		idx := strings.IndexByte(combined, '\n')
		if idx < 0 {
			trimmed := strings.TrimSpace(combined)
			if isPotentialOpenAIDoneLinePrefix(trimmed) {
				return out.Bytes(), combined, sawFinishReason || finishInserted, sawDone
			}
			if strings.HasPrefix(trimmed, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				if data == "[DONE]" {
					sawDone = true
					if !alreadySawFinishReason && !sawFinishReason && !finishInserted {
						out.Write(openAIStreamFinishChunk(model))
						finishInserted = true
					}
				} else if openAIChunkHasFinishReason([]byte(data)) {
					sawFinishReason = true
				}
			}
			out.WriteString(combined)
			return out.Bytes(), "", sawFinishReason || finishInserted, sawDone
		}
		line := combined[:idx+1]
		combined = combined[idx+1:]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data == "[DONE]" {
				sawDone = true
				if !alreadySawFinishReason && !sawFinishReason && !finishInserted {
					out.Write(openAIStreamFinishChunk(model))
					finishInserted = true
				}
			} else if openAIChunkHasFinishReason([]byte(data)) {
				sawFinishReason = true
			}
		}
		out.WriteString(line)
	}
	return out.Bytes(), "", sawFinishReason || finishInserted, sawDone
}

func isPotentialOpenAIDoneLinePrefix(line string) bool {
	if !strings.HasPrefix(line, "data:") {
		return false
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" {
		return true
	}
	return strings.HasPrefix("[DONE]", data)
}

func openAIChunkHasFinishReason(data []byte) bool {
	var root struct {
		Choices []struct {
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return false
	}
	for _, choice := range root.Choices {
		if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
			return true
		}
	}
	return false
}

func openAIStreamFinishChunk(model string) []byte {
	if strings.TrimSpace(model) == "" {
		model = "grok"
	}
	payload := map[string]any{
		"id":      "chatcmpl-grok-final",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": "stop",
		}},
	}
	encoded, _ := json.Marshal(payload)
	return []byte("data: " + string(encoded) + "\n\n")
}

func extractModelFromResponse(payload []byte) string {
	var resp map[string]any
	if err := json.Unmarshal(payload, &resp); err != nil {
		return ""
	}
	if model, ok := resp["model"].(string); ok {
		return model
	}
	return ""
}

func extractModelFromStream(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if model := extractModelFromResponse([]byte(data)); model != "" {
			return model
		}
	}
	return ""
}
