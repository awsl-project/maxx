// Package fal implements a first-class provider adapter for fal (fal.ai), a
// queue-based inference platform that is NOT OpenAI-compatible. fal routes models
// by URL PATH (e.g. https://fal.run/fal-ai/flux/dev) and authenticates with an
// "Authorization: Key <id>:<secret>" header (NOT Bearer). Unlike the openrouter/
// zai adapters — which merely synthesize a custom config and delegate to the
// custom proxy core — fal needs a genuine TRANSLATION layer: maxx makes fal look
// like OpenAI (images) and new-api (async video) to clients, translating request
// and response shapes in both directions and calling fal directly.
//
// Two surfaces, two fal shapes:
//
//   - Image (ClientTypeOpenAI, POST /v1/images/generations): fal image models are
//     SYNCHRONOUS. maxx POSTs to https://fal.run/{model} (blocks ~1-2s) and
//     translates the OpenAI images request/response on the fly.
//   - Video (ClientTypeVideo, POST /v1/video/generations submit + GET
//     /v1/video/generations/{task_id} poll): fal video models are ASYNC via the
//     queue at https://queue.fal.run/{model}. maxx encodes the fal result URL into
//     the returned task_id so polling stays STATELESS (maxx stores nothing), and
//     translates to the new-api video envelope clients already speak.
//
// The mapped model (executor.mapModel → e.g. "fal-ai/flux/dev") is the URL path
// segment; slashes are fine in the path.
package fal

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// fal's fixed API roots. Image models are served synchronously from fal.run; the
// async queue (video) lives on queue.fal.run. Both are redirectable via env vars
// so tests can point at a mock upstream.
const (
	defaultSyncBaseURL  = "https://fal.run"
	defaultQueueBaseURL = "https://queue.fal.run"
)

func resolveSyncBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("MAXX_FAL_BASE_URL")); v != "" {
		return v
	}
	return defaultSyncBaseURL
}

func resolveQueueBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("MAXX_FAL_QUEUE_BASE_URL")); v != "" {
		return v
	}
	return defaultQueueBaseURL
}

func init() {
	provider.RegisterAdapterFactory("fal", NewAdapter)
}

// Adapter is a first-class fal provider that translates maxx's OpenAI (image) and
// new-api (video) surfaces to fal's queue-based API and back.
type Adapter struct {
	apiKey   string
	provider *domain.Provider
	client   *http.Client
}

// NewAdapter builds a fal adapter from the fal config (the full "id:secret" key).
func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Fal == nil {
		return nil, fmt.Errorf("provider %s missing fal config", p.Name)
	}
	return &Adapter{
		apiKey:   p.Config.Fal.APIKey,
		provider: p,
		client:   &http.Client{Timeout: 10 * time.Minute},
	}, nil
}

// SupportedClientTypes reports fal's native surfaces: OpenAI (images) and Video.
func (a *Adapter) SupportedClientTypes() []domain.ClientType {
	return domain.CanonicalSupportedClientTypes("fal", a.provider.SupportedClientTypes)
}

// Execute routes to the image or video translator based on the inbound client
// type. Any client-supplied auth header is stripped before forwarding — only
// fal's provider key ("Authorization: Key <id:secret>") reaches upstream.
func (a *Adapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	switch flow.GetClientType(c) {
	case domain.ClientTypeVideo:
		return a.executeVideo(c)
	default:
		// ClientTypeOpenAI (images). fal is only advertised for openai+video, so
		// routing never sends other client types here.
		return a.executeImage(c)
	}
}

// authHeader injects fal's non-Bearer auth. fal expects the FULL id:secret string
// verbatim after "Key ".
func (a *Adapter) authHeader(req *http.Request) {
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Key "+a.apiKey)
	}
}

// doJSON performs an authenticated fal HTTP call and returns the raw body. It
// classifies transport failures and >=400 responses as provider/request errors so
// routing can retry or cool down appropriately.
func (a *Adapter) doJSON(ctx context.Context, method, url string, body []byte) ([]byte, int, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "failed to create fal request")
		proxyErr.Scope = domain.ScopeEndpoint
		proxyErr.Reason = domain.CooldownReasonServerError
		return nil, 0, proxyErr
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	a.authHeader(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, 0, domain.NewUpstreamConnectionError("failed to connect to fal")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		proxyErr := domain.NewProxyErrorWithMessage(
			fmt.Errorf("fal upstream error: %s", string(respBody)),
			isRetryableStatus(resp.StatusCode),
			fmt.Sprintf("fal returned status %d", resp.StatusCode),
		)
		proxyErr.HTTPStatusCode = resp.StatusCode
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			proxyErr.Scope = domain.ScopeKey
			proxyErr.Reason = domain.CooldownReasonAuthFailure
			proxyErr.Retryable = false
		} else if resp.StatusCode >= 500 {
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Reason = domain.CooldownReasonServerError
		} else {
			proxyErr.Scope = domain.ScopeRequest
			proxyErr.Retryable = false
		}
		return respBody, resp.StatusCode, proxyErr
	}
	return respBody, resp.StatusCode, nil
}

func isRetryableStatus(code int) bool {
	switch code {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// writeJSON writes a JSON response to the client, mirroring the event bookkeeping
// the custom core does so request logging stays consistent.
func writeJSON(c *flow.Ctx, status int, body []byte) error {
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status:  status,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    string(body),
		})
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(status)
	if _, err := c.Writer.Write(body); err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "client disconnected")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}
	return nil
}

func (a *Adapter) sendRequestInfo(c *flow.Ctx, method, url string, body []byte) {
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  method,
			URL:     url,
			Headers: map[string]string{"Authorization": "Key ***"},
			Body:    string(body),
		})
	}
}

// base64url without padding — used to encode the fal result URL into the video
// task id so polling is stateless (maxx recovers everything it needs from the id).
var b64url = base64.RawURLEncoding

func encodeTaskID(parts ...string) string {
	return b64url.EncodeToString([]byte(strings.Join(parts, "\n")))
}

func decodeTaskID(taskID string) ([]string, error) {
	raw, err := b64url.DecodeString(taskID)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(raw), "\n"), nil
}

// b64Std standard-base64-encodes bytes (with padding) — the encoding OpenAI's
// b64_json image field uses.
func b64Std(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
