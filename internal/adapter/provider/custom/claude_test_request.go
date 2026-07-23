package custom

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

// NewClaudeMessagesTestRequest builds the same upstream Claude messages request shape
// used by CustomAdapter for a non-stream Claude request. It is intentionally small:
// callers provide the already model-mapped Claude payload and the resolved Claude
// base URL, while this helper owns the custom-provider body/header compatibility
// contract (disguise mode, Claude Code headers, Bedrock compatibility, auth style,
// beta extraction, and OAuth tool prefixing).
func NewClaudeMessagesTestRequest(ctx context.Context, provider *domain.Provider, baseURL string, payload []byte) (*http.Request, error) {
	endpoint := addClaudeQueryParams(buildUpstreamURL(strings.TrimRight(strings.TrimSpace(baseURL), "/"), "/v1/messages"))
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	customCfg := provider.Config.Custom
	apiKey := customCfg.APIKey
	clientReq := newClaudeBatchSyntheticClientRequest()
	clientUA := clientReq.Header.Get("User-Agent")
	requestBody, extraBetas := processClaudeRequestBody(payload, clientUA, customCfg)
	useAPIKey := shouldUseClaudeAPIKey(apiKey, clientReq)
	if isClaudeOAuthToken(apiKey) {
		requestBody = applyClaudeToolPrefix(requestBody, claudeToolPrefix)
	}

	effectiveDisguise := customCfg.ResolveDisguise()
	disguiseType := ""
	if effectiveDisguise != nil {
		disguiseType = strings.ToLower(strings.TrimSpace(effectiveDisguise.Type))
	}
	switch disguiseType {
	case domain.DisguiseTypeBedrock:
		applyBedrockCompatHeaders(upstreamReq, clientReq, apiKey, false)
	case domain.DisguiseTypeNone:
		upstreamReq.Header = make(http.Header)
		copyHeadersFiltered(upstreamReq.Header, clientReq.Header)
		if len(extraBetas) > 0 {
			upstreamReq.Header.Set(
				"Anthropic-Beta",
				mergeBetaList(upstreamReq.Header.Get("Anthropic-Beta"), extraBetas),
			)
		}
		setClaudeAuthForURL(upstreamReq, apiKey, useAPIKey)
	default:
		applyClaudeHeaders(upstreamReq, clientReq, apiKey, useAPIKey, extraBetas, false)
	}

	upstreamReq.Body = io.NopCloser(bytes.NewReader(requestBody))
	upstreamReq.ContentLength = int64(len(requestBody))
	return upstreamReq, nil
}

func newClaudeBatchSyntheticClientRequest() *http.Request {
	req, _ := http.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultClaudeUserAgent)
	req.Header.Set("Anthropic-Version", defaultAnthropicVersion)
	return req
}
