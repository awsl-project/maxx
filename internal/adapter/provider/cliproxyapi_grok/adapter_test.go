package cliproxyapi_grok

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type fakeGrokExecutor struct {
	chunks        []executor.StreamChunk
	requests      []executor.Request
	streamOptions []executor.Options
}

func (f *fakeGrokExecutor) Execute(context.Context, *auth.Auth, executor.Request, executor.Options) (executor.Response, error) {
	return executor.Response{}, errors.New("unexpected non-stream execution")
}

func (f *fakeGrokExecutor) ExecuteStream(_ context.Context, _ *auth.Auth, req executor.Request, opts executor.Options) (*executor.StreamResult, error) {
	f.requests = append(f.requests, req)
	f.streamOptions = append(f.streamOptions, opts)
	ch := make(chan executor.StreamChunk, len(f.chunks))
	for _, chunk := range f.chunks {
		ch <- chunk
	}
	close(ch)
	return &executor.StreamResult{Chunks: ch}, nil
}

func TestNewAdapterAcceptsCPAExportedXAIOAuthJSONShape(t *testing.T) {
	provider := &domain.Provider{
		ID:   42,
		Type: "grok",
		Name: "Grok Test",
		Config: &domain.ProviderConfig{Grok: &domain.ProviderConfigGrok{
			Type:          "xai",
			AuthKind:      "oauth",
			Email:         "xai39b5bb@jh.actionvspot.com",
			Sub:           "88ca5464-ae36-48c5-a7b8-de306101d07f",
			AccessToken:   "access-token",
			RefreshToken:  "refresh-token",
			IDToken:       "id-token",
			TokenType:     "Bearer",
			ExpiresIn:     21600,
			Expired:       "2026-07-11T21:01:55Z",
			LastRefresh:   "2026-07-11T15:01:56Z",
			RedirectURI:   "http://127.0.0.1:56121/callback",
			TokenEndpoint: "https://auth.x.ai/oauth2/token",
			BaseURL:       "https://cli-chat-proxy.grok.com/v1",
			Headers: map[string]string{
				"X-XAI-Token-Auth":      "xai-grok-cli",
				"x-grok-client-version": "0.2.93",
			},
		}},
	}

	adapter, err := NewAdapter(provider)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	grok, ok := adapter.(*CLIProxyAPIGrokAdapter)
	if !ok {
		t.Fatalf("adapter type = %T, want *CLIProxyAPIGrokAdapter", adapter)
	}
	if got := grok.authObj.Provider; got != "xai" {
		t.Fatalf("auth provider = %q, want xai", got)
	}
	if got := grok.authObj.Metadata["access_token"]; got != "access-token" {
		t.Fatalf("access_token metadata = %v", got)
	}
	if got := grok.authObj.Metadata["refresh_token"]; got != "refresh-token" {
		t.Fatalf("refresh_token metadata = %v", got)
	}
	if got := grok.authObj.Metadata["base_url"]; got != "https://cli-chat-proxy.grok.com/v1" {
		t.Fatalf("base_url metadata = %v", got)
	}
}

func TestEnsureOpenAIStreamFinishBeforeDoneInsertsFinishReason(t *testing.T) {
	input := []byte("data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"model\":\"grok-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n")

	out, sawFinishReason, sawDone := ensureOpenAIStreamFinishBeforeDone(input, "grok-test", false)
	body := string(out)
	if !sawFinishReason {
		t.Fatalf("sawFinishReason = false, want true; body=%s", body)
	}
	if !sawDone {
		t.Fatalf("sawDone = false, want true; body=%s", body)
	}
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("stream body missing terminal finish_reason stop: %s", body)
	}
	finishIdx := strings.Index(body, `"finish_reason":"stop"`)
	doneIdx := strings.Index(body, "data: [DONE]")
	if finishIdx < 0 || doneIdx < 0 || finishIdx > doneIdx {
		t.Fatalf("finish_reason must be emitted before [DONE]; body=%s", body)
	}
}

func TestEnsureOpenAIStreamFinishBeforeDonePassesThroughChunkWithoutNewline(t *testing.T) {
	input := []byte(`data: {"id":"chunk-1","object":"chat.completion.chunk","model":"grok-test","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`)
	out, pending, sawFinishReason, sawDone := ensureOpenAIStreamFinishBeforeDoneWithPending("", input, "grok-test", false)
	if pending != "" {
		t.Fatalf("pending = %q, want empty", pending)
	}
	if sawFinishReason || sawDone {
		t.Fatalf("sawFinishReason=%v sawDone=%v, want false/false", sawFinishReason, sawDone)
	}
	if string(out) != string(input) {
		t.Fatalf("output changed: got %q want %q", string(out), string(input))
	}
}

func TestEnsureOpenAIStreamFinishBeforeDoneHandlesDoneSplitAcrossChunks(t *testing.T) {
	first := []byte("data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"model\":\"grok-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\ndata: [")
	second := []byte("DONE]\n\n")

	out1, pending, sawFinishReason, sawDone := ensureOpenAIStreamFinishBeforeDoneWithPending("", first, "grok-test", false)
	out2, pending, chunkSawFinishReason, chunkSawDone := ensureOpenAIStreamFinishBeforeDoneWithPending(pending, second, "grok-test", sawFinishReason)
	if chunkSawFinishReason {
		sawFinishReason = true
	}
	if chunkSawDone {
		sawDone = true
	}
	body := string(append(out1, out2...))
	if pending != "" {
		t.Fatalf("pending = %q, want empty; body=%s", pending, body)
	}
	if !sawDone {
		t.Fatalf("sawDone = false, want true; body=%s", body)
	}
	if !sawFinishReason {
		t.Fatalf("sawFinishReason = false, want true; body=%s", body)
	}
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("split [DONE] stream missing terminal finish_reason stop: %s", body)
	}
}

func TestEnsureOpenAIStreamFinishBeforeDoneDoesNotDuplicateFinishReason(t *testing.T) {
	input := []byte("data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"model\":\"grok-test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")

	out, sawFinishReason, sawDone := ensureOpenAIStreamFinishBeforeDone(input, "grok-test", false)
	body := string(out)
	if !sawFinishReason || !sawDone {
		t.Fatalf("sawFinishReason=%v sawDone=%v body=%s", sawFinishReason, sawDone, body)
	}
	if count := strings.Count(body, `"finish_reason":"stop"`); count != 1 {
		t.Fatalf("finish_reason stop count = %d, want 1; body=%s", count, body)
	}
}

func TestExecuteStreamReturnsClientVisibleGrokContentAndFinishReason(t *testing.T) {
	provider := &domain.Provider{
		ID:   42,
		Type: "grok",
		Name: "Grok Test",
		Config: &domain.ProviderConfig{Grok: &domain.ProviderConfigGrok{
			Type:         "xai",
			AuthKind:     "oauth",
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
		}},
	}
	adapter, err := NewAdapter(provider)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	grok := adapter.(*CLIProxyAPIGrokAdapter)
	fakeExec := &fakeGrokExecutor{chunks: []executor.StreamChunk{
		{Payload: []byte(`data: {"id":"chunk-1","object":"chat.completion.chunk","model":"grok-test","choices":[{"index":0,"delta":{"content":"hel"},"finish_reason":null}]}`)},
		{Payload: []byte(`data: {"id":"chunk-2","object":"chat.completion.chunk","model":"grok-test","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":null}]}` + "\n\n")},
		{Payload: []byte("data: [")},
		{Payload: []byte("DONE]\n\n")},
	}}
	grok.executor = fakeExec

	body := []byte(`{"model":"grok-test","stream":true,"messages":[{"role":"user","content":"say hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	c := flow.NewCtx(rec, req)
	c.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	c.Set(flow.KeyRequestBody, body)
	c.Set(flow.KeyRequestModel, "grok-test")
	c.Set(flow.KeyMappedModel, "grok-test")
	c.Set(flow.KeyIsStream, true)
	c.Set(flow.KeyRequestURI, "/v1/chat/completions")

	if err := grok.Execute(c, provider); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, got)
	}
	contentIdx := strings.Index(got, `"content":"hel"`)
	finishIdx := strings.Index(got, `"finish_reason":"stop"`)
	doneIdx := strings.Index(got, "data: [DONE]")
	if contentIdx < 0 {
		t.Fatalf("client-visible content missing: %s", got)
	}
	if finishIdx < 0 || doneIdx < 0 || !(contentIdx < finishIdx && finishIdx < doneIdx) {
		t.Fatalf("want content before finish_reason before [DONE]; body=%s", got)
	}
	if len(fakeExec.requests) != 1 || fakeExec.requests[0].Model != "grok-test" {
		t.Fatalf("executor request model = %#v, want grok-test", fakeExec.requests)
	}
}

func TestExecuteStreamReturnsClientVisibleContentForRepresentativeGrokModels(t *testing.T) {
	models := []string{
		"grok-3",
		"grok-3-mini",
		"grok-3-mini-fast",
		"grok-4",
		"grok-4.1",
		"grok-4.3",
		"grok-4.5",
		"grok-latest",
	}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			provider := &domain.Provider{
				ID:   42,
				Type: "grok",
				Name: "Grok Test",
				Config: &domain.ProviderConfig{Grok: &domain.ProviderConfigGrok{
					Type:         "xai",
					AuthKind:     "oauth",
					AccessToken:  "access-token",
					RefreshToken: "refresh-token",
				}},
			}
			adapter, err := NewAdapter(provider)
			if err != nil {
				t.Fatalf("NewAdapter() error = %v", err)
			}
			grok := adapter.(*CLIProxyAPIGrokAdapter)
			fakeExec := &fakeGrokExecutor{chunks: []executor.StreamChunk{
				{Payload: []byte(`data: {"id":"chunk-1","object":"chat.completion.chunk","model":"` + model + `","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`)},
				{Payload: []byte("data: [")},
				{Payload: []byte("DONE]\n\n")},
			}}
			grok.executor = fakeExec

			body := []byte(`{"model":"` + model + `","stream":true,"messages":[{"role":"user","content":"say ok"}]}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
			rec := httptest.NewRecorder()
			c := flow.NewCtx(rec, req)
			c.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
			c.Set(flow.KeyRequestBody, body)
			c.Set(flow.KeyRequestModel, model)
			c.Set(flow.KeyMappedModel, model)
			c.Set(flow.KeyIsStream, true)
			c.Set(flow.KeyRequestURI, "/v1/chat/completions")

			if err := grok.Execute(c, provider); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			got := rec.Body.String()
			if !strings.Contains(got, `"content":"ok"`) {
				t.Fatalf("client-visible content missing: %s", got)
			}
			if !strings.Contains(got, `"finish_reason":"stop"`) || !strings.Contains(got, "data: [DONE]") {
				t.Fatalf("client-visible stream missing finish terminator: %s", got)
			}
			if len(fakeExec.requests) != 1 || fakeExec.requests[0].Model != model {
				t.Fatalf("executor request model = %#v, want %s", fakeExec.requests, model)
			}
		})
	}
}

func TestGrokImagesRequestUsesOpenAIImageSourceFormat(t *testing.T) {
	if !isOpenAIImagesRequest("/v1/images/generations") {
		t.Fatalf("/v1/images/generations should be treated as an OpenAI Images request")
	}
	if !isOpenAIImagesRequest("/images/generations?source=test") {
		t.Fatalf("/images/generations?source=test should be treated as an OpenAI Images request")
	}
	if isOpenAIImagesRequest("/v1/chat/completions") {
		t.Fatalf("/v1/chat/completions must not be treated as an OpenAI Images request")
	}
}

func TestGrokRequestMetadataCarriesRequestPath(t *testing.T) {
	metadata := requestMetadata("/v1/images/generations?source=test")
	if got := metadata[executor.RequestPathMetadataKey]; got != "/v1/images/generations" {
		t.Fatalf("request_path metadata = %v, want /v1/images/generations", got)
	}
}
