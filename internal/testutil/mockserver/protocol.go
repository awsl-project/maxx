package mockserver

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// DetectProtocol determines the API protocol from the request path.
func DetectProtocol(r *http.Request) Protocol {
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/v1/messages"):
		return ProtocolClaude
	case strings.HasPrefix(path, "/v1/chat/completions"):
		return ProtocolOpenAI
	case strings.HasPrefix(path, "/v1beta/models/"):
		return ProtocolGemini
	case strings.HasPrefix(path, "/v1/responses") || strings.HasPrefix(path, "/responses"):
		return ProtocolCodex
	default:
		return ProtocolOpenAI
	}
}

// ExtractModel extracts the model name from the request body or URL path.
func ExtractModel(protocol Protocol, body []byte, urlPath string) string {
	switch protocol {
	case ProtocolGemini:
		// Model is in URL: /v1beta/models/{model}:generateContent
		if idx := strings.Index(urlPath, "/v1beta/models/"); idx >= 0 {
			rest := urlPath[idx+len("/v1beta/models/"):]
			if colon := strings.Index(rest, ":"); colon > 0 {
				return rest[:colon]
			}
			return rest
		}
		return "gemini-2.5-flash"
	default:
		if model := gjson.GetBytes(body, "model").String(); model != "" {
			return model
		}
		return "mock-model"
	}
}
