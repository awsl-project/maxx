package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/tidwall/gjson"
)

// tinyPNGDataURL is a 1x1 PNG as a data URL — the shape OpenRouter returns a
// generated image in (message.images[].image_url.url).
const tinyPNGDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// openRouterLikeConversionAdapter mimics an OpenRouter provider: it natively
// speaks Claude AND OpenAI (but not the Gemini API), captures the request it
// receives, and returns a canned OpenAI chat.completion.
type openRouterLikeConversionAdapter struct {
	calls           int
	seenClientType  domain.ClientType
	seenRequestURI  string
	seenRequestBody []byte
	responseBody    string
}

func (a *openRouterLikeConversionAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI}
}

func (a *openRouterLikeConversionAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	a.calls++
	a.seenClientType = flow.GetClientType(c)
	a.seenRequestURI = flow.GetRequestURI(c)
	a.seenRequestBody = append([]byte(nil), flow.GetRequestBody(c)...)

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, err := c.Writer.Write([]byte(a.responseBody))
	return err
}

// A native Gemini image request routed to an OpenRouter-like provider (speaks
// Claude + OpenAI, not Gemini) must convert through OpenAI, because only the
// Gemini<->OpenAI pair carries a generated image. Converting through Claude
// (Anthropic Messages) silently drops it. This locks the full dispatch path:
// target selection (GetPreferredTargetType) + request/response conversion.
func TestDispatchGeminiImageRequestConvertsThroughOpenAINotClaude(t *testing.T) {
	adapter := &openRouterLikeConversionAdapter{responseBody: `{
		"id":"gen-1","object":"chat.completion","created":1700000000,
		"model":"google/gemini-2.5-flash-image","provider":"Google",
		"choices":[{"index":0,"message":{"role":"assistant","content":"here you go",
			"images":[{"type":"image_url","image_url":{"url":"` + tinyPNGDataURL + `"}}]},
			"finish_reason":"stop"}],
		"usage":{"prompt_tokens":6,"completion_tokens":1300,"total_tokens":1306}
	}`}

	geminiReq := `{"contents":[{"role":"user","parts":[{"text":"a red apple"}]}],` +
		`"generationConfig":{"responseModalities":["IMAGE"]}}`
	c, proxyRepo, attemptRepo := newGeminiImageDispatchCtx(t, geminiReq, adapter)
	e := &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      attemptRepo,
		modelMappingRepo: &stubModelMappingRepo{},
		settingsRepo:     &stubExecutorSettingsRepo{},
		converter:        converter.GetGlobalRegistry(),
	}

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}

	// The upstream must be reached as OpenAI on the chat endpoint — NOT Claude.
	if adapter.seenClientType != domain.ClientTypeOpenAI {
		t.Fatalf("upstream client type = %s, want openai (must not route Gemini image through Claude)", adapter.seenClientType)
	}
	if adapter.seenRequestURI != "/v1/chat/completions" {
		t.Fatalf("upstream URI = %q, want /v1/chat/completions", adapter.seenRequestURI)
	}
	// The converted request must ask OpenRouter for image output.
	if mods := gjson.GetBytes(adapter.seenRequestBody, "modalities"); !mods.Exists() {
		t.Fatalf("converted request dropped modalities: %s", adapter.seenRequestBody)
	}

	// The client must receive the generated image back as a Gemini inlineData part.
	body := c.Writer.(*httptest.ResponseRecorder).Body.Bytes()
	mime := gjson.GetBytes(body, "candidates.0.content.parts.#(inlineData.mimeType!=).inlineData.mimeType").String()
	data := gjson.GetBytes(body, "candidates.0.content.parts.#(inlineData.data!=).inlineData.data").String()
	if mime != "image/png" {
		t.Fatalf("client did not receive inlineData image (mime=%q); body=%s", mime, body)
	}
	if data == "" {
		t.Fatalf("generated image dropped on the way back to the Gemini client; body=%s", body)
	}
	// The text preamble must survive too.
	if !strings.Contains(string(body), "here you go") {
		t.Fatalf("text preamble lost: %s", body)
	}

	if len(attemptRepo.updated) == 0 || attemptRepo.updated[len(attemptRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed attempt, got %#v", attemptRepo.updated)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed proxy request, got %#v", proxyRepo.updated)
	}
}

func newGeminiImageDispatchCtx(t *testing.T, requestBody string, adapter *openRouterLikeConversionAdapter) (*flow.Ctx, *recordingProxyRequestRepo, *recordingAttemptRepo) {
	t.Helper()
	proxyRepo := &recordingProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	rec := httptest.NewRecorder()
	const uri = "/v1beta/models/gemini-2.5-flash-image:generateContent"
	req := httptest.NewRequest(http.MethodPost, uri, strings.NewReader(requestBody)).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:           202,
		TenantID:     domain.DefaultTenantID,
		ClientType:   domain.ClientTypeGemini,
		RequestModel: "gemini-2.5-flash-image",
		Status:       "IN_PROGRESS",
		StartTime:    time.Now(),
	}
	state := &execState{
		ctx:                 context.Background(),
		proxyReq:            proxyReq,
		tenantID:            domain.DefaultTenantID,
		clientType:          domain.ClientTypeGemini,
		requestModel:        "gemini-2.5-flash-image",
		isStream:            false,
		requestBody:         []byte(requestBody),
		originalRequestBody: []byte(requestBody),
		requestHeaders:      http.Header{"Content-Type": []string{"application/json"}},
		requestURI:          uri,
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 32, TenantID: domain.DefaultTenantID, ProviderID: 42, ClientType: domain.ClientTypeGemini},
				Provider: &domain.Provider{
					ID:                   42,
					TenantID:             domain.DefaultTenantID,
					Type:                 "openrouter",
					Name:                 "openrouter-like",
					SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI},
				},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	return c, proxyRepo, attemptRepo
}
