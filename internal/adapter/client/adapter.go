package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

// RequestInfo contains extracted request information
type RequestInfo struct {
	SessionID    string
	RequestModel string
}

// Adapter handles client type detection and request parsing
type Adapter struct{}

// NewAdapter creates a new client adapter
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Gemini URL patterns
var geminiModelPattern = regexp.MustCompile(`/v1beta/models/([^/:]+)`)
var geminiInternalPattern = regexp.MustCompile(`/v1internal/models/([^/:]+)`)

// Match detects the client type from the request
func (a *Adapter) Match(req *http.Request) (domain.ClientType, bool) {
	// First layer: endpoint detection
	path := req.URL.Path

	switch {
	case strings.HasPrefix(path, "/v1/messages"):
		return domain.ClientTypeClaude, true
	case strings.HasPrefix(path, "/responses"):
		return domain.ClientTypeCodex, true
	case strings.HasPrefix(path, "/v1/responses"):
		return domain.ClientTypeCodex, true
	case strings.HasPrefix(path, "/v1/chat/completions"):
		return domain.ClientTypeOpenAI, true
	case strings.HasPrefix(path, "/v1/images/"):
		// OpenAI Images API (generations/edits). Body carries no messages/input,
		// so body-detection can't classify it — key off the path.
		return domain.ClientTypeOpenAI, true
	case strings.HasPrefix(path, "/v1beta/models/"):
		return domain.ClientTypeGemini, true
	case strings.HasPrefix(path, "/v1internal/models/"):
		return domain.ClientTypeGemini, true
	}

	// Second layer: body detection (fallback)
	return a.detectFromBody(req)
}

func (a *Adapter) detectFromBody(req *http.Request) (domain.ClientType, bool) {
	if req.Body == nil {
		return "", false
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", false
	}
	// Restore body for later use
	req.Body = io.NopCloser(bytes.NewReader(body))

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", false
	}

	// Check for Gemini format
	if _, ok := data["contents"]; ok {
		if _, hasRequest := data["request"]; !hasRequest {
			return domain.ClientTypeGemini, true
		}
	}

	// Check for Gemini CLI (envelope)
	if _, ok := data["request"]; ok {
		return domain.ClientTypeGemini, true
	}

	// Check for Codex (Response API)
	if _, ok := data["input"]; ok {
		return domain.ClientTypeCodex, true
	}

	// Check for Claude vs OpenAI
	if _, ok := data["messages"]; ok {
		// Claude has system as array or string at top level
		if _, hasSystem := data["system"]; hasSystem {
			return domain.ClientTypeClaude, true
		}
		return domain.ClientTypeOpenAI, true
	}

	return "", false
}

// ExtractInfo extracts session ID and model from the request
func (a *Adapter) ExtractInfo(req *http.Request, clientType domain.ClientType) (*RequestInfo, []byte, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))

	info := &RequestInfo{}

	// Extract model
	info.RequestModel = a.extractModel(req, clientType, body)

	// Extract session ID
	info.SessionID = a.extractSessionID(req, clientType, body)

	return info, body, nil
}

func (a *Adapter) extractModel(req *http.Request, clientType domain.ClientType, body []byte) string {
	// Delegate to the exported ExtractModel (same Gemini-URL → JSON → multipart
	// logic) so ExtractInfo stays multipart-aware (e.g. images/edits) instead of
	// silently returning "" for non-JSON bodies. Note the arg-order difference.
	return a.ExtractModel(req, body, clientType)
}

func (a *Adapter) extractSessionID(req *http.Request, clientType domain.ClientType, body []byte) string {
	// 1. For Codex client, try Session_id header first
	if clientType == domain.ClientTypeCodex {
		if sid := req.Header.Get("Session_id"); sid != "" {
			return sid
		}
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err == nil {
		// 2. For Codex client, try previous_response_id or prompt_cache_key
		if clientType == domain.ClientTypeCodex {
			// First try previous_response_id (used for conversation tracking in Codex)
			if prevID, ok := data["previous_response_id"].(string); ok && prevID != "" {
				return prevID
			}
			// Then try prompt_cache_key (used for session identification)
			if cacheKey, ok := data["prompt_cache_key"].(string); ok && cacheKey != "" {
				return cacheKey
			}
		}

		// 3. Try metadata.session_id or metadata.user_id (Claude)
		if metadata, ok := data["metadata"].(map[string]interface{}); ok {
			// First try explicit session_id
			if sid, ok := metadata["session_id"].(string); ok && sid != "" {
				return sid
			}
			// Then try user_id (Claude Code format: "user_{hash}_account__session_{uuid}")
			if userID, ok := metadata["user_id"].(string); ok && userID != "" {
				const sessionMarker = "_session_"
				if idx := strings.LastIndex(userID, sessionMarker); idx != -1 {
					return userID[idx+len(sessionMarker):]
				}
				// Fallback: return full user_id if no _session_ marker
				return userID
			}
		}
	}

	// 4. Try Header X-Session-Id
	if sid := req.Header.Get("X-Session-Id"); sid != "" {
		return sid
	}

	// 5. Generate deterministic session ID from request characteristics
	return a.generateSessionID(req, body)
}

func (a *Adapter) generateSessionID(req *http.Request, body []byte) string {
	// Use a combination of:
	// - Authorization header (identifies the user/key)
	// - User-Agent
	// - Some stable request characteristics

	h := sha256.New()

	// Auth header is the primary identifier
	if auth := req.Header.Get("Authorization"); auth != "" {
		h.Write([]byte(auth))
	}
	if key := req.Header.Get("X-Api-Key"); key != "" {
		h.Write([]byte(key))
	}

	// Add user agent for differentiation
	h.Write([]byte(req.UserAgent()))

	// Add remote address (without port for stability)
	remoteIP := strings.Split(req.RemoteAddr, ":")[0]
	h.Write([]byte(remoteIP))

	return "session-" + hex.EncodeToString(h.Sum(nil))[:16]
}

// DetectClientType detects the client type from the request
func (a *Adapter) DetectClientType(req *http.Request, body []byte) domain.ClientType {
	// First layer: endpoint detection
	path := req.URL.Path

	switch {
	case strings.HasPrefix(path, "/v1/messages"):
		return domain.ClientTypeClaude
	case strings.HasPrefix(path, "/v1/responses"):
		return domain.ClientTypeCodex
	case strings.HasPrefix(path, "/responses"):
		return domain.ClientTypeCodex
	case strings.HasPrefix(path, "/v1/chat/completions"):
		return domain.ClientTypeOpenAI
	case strings.HasPrefix(path, "/v1/images/"):
		// OpenAI Images API (generations/edits). Body carries no messages/input,
		// so body-detection can't classify it — key off the path.
		return domain.ClientTypeOpenAI
	case strings.HasPrefix(path, "/v1beta/models/"):
		return domain.ClientTypeGemini
	case strings.HasPrefix(path, "/v1internal/models/"):
		return domain.ClientTypeGemini
	}

	// Second layer: body detection (fallback)
	detected := a.detectFromBodyBytes(body)
	if detected == domain.ClientTypeOpenAI && isClaudeUserAgent(req.UserAgent()) {
		return domain.ClientTypeClaude
	}
	return detected
}

func (a *Adapter) detectFromBodyBytes(body []byte) domain.ClientType {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	// Check for Gemini format
	if _, ok := data["contents"]; ok {
		if _, hasRequest := data["request"]; !hasRequest {
			return domain.ClientTypeGemini
		}
	}

	// Check for Gemini CLI (envelope)
	if _, ok := data["request"]; ok {
		return domain.ClientTypeGemini
	}

	// Check for Codex (Response API)
	if _, ok := data["input"]; ok {
		return domain.ClientTypeCodex
	}

	// Check for Claude vs OpenAI
	if _, ok := data["messages"]; ok {
		// Claude has system as array or string at top level
		if _, hasSystem := data["system"]; hasSystem {
			return domain.ClientTypeClaude
		}
		return domain.ClientTypeOpenAI
	}

	return ""
}

func isClaudeUserAgent(userAgent string) bool {
	return strings.HasPrefix(userAgent, "claude-cli")
}

// ExtractModel extracts the model from the request (URL path for Gemini, body for others)
func (a *Adapter) ExtractModel(req *http.Request, body []byte, clientType domain.ClientType) string {
	// For Gemini, try URL path first
	if clientType == domain.ClientTypeGemini {
		path := req.URL.Path
		if matches := geminiModelPattern.FindStringSubmatch(path); len(matches) > 1 {
			return matches[1]
		}
		if matches := geminiInternalPattern.FindStringSubmatch(path); len(matches) > 1 {
			return matches[1]
		}
	}

	// Try JSON body (chat/completions, images/generations, messages, ...).
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err == nil {
		if model, ok := data["model"].(string); ok {
			return model
		}
		return ""
	}

	// Non-JSON body: OpenAI images/edits is multipart/form-data with the model
	// as a form field. Without this, edits requests would price at model="" → cost 0.
	if model := modelFromMultipartForm(req, body); model != "" {
		return model
	}

	return ""
}

// modelFromMultipartForm pulls the "model" form field out of a multipart/form-data
// body (OpenAI images/edits). It reads parts sequentially and stops at "model",
// skipping the (potentially large) uploaded image without buffering it whole.
func modelFromMultipartForm(req *http.Request, body []byte) string {
	if req == nil {
		return ""
	}
	mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return ""
	}
	boundary := params["boundary"]
	if boundary == "" {
		return ""
	}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			return ""
		}
		if part.FormName() != "model" {
			_ = part.Close()
			continue
		}
		buf := make([]byte, 256)
		n, _ := io.ReadFull(part, buf)
		_ = part.Close()
		return strings.TrimSpace(string(buf[:n]))
	}
}

// ExtractSessionID extracts the session ID from request
func (a *Adapter) ExtractSessionID(req *http.Request, body []byte, clientType domain.ClientType) string {
	return a.extractSessionID(req, clientType, body)
}

// IsStreamRequest checks if the request is for streaming
// For Gemini: check URL path for "streamGenerateContent"
// For Claude/OpenAI: check body for "stream: true"
func (a *Adapter) IsStreamRequest(req *http.Request, body []byte) bool {
	path := req.URL.Path

	// Gemini uses URL path to indicate streaming
	if strings.Contains(path, "streamGenerateContent") {
		return true
	}

	// Claude/OpenAI use body field
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return false
	}

	if stream, ok := data["stream"].(bool); ok {
		return stream
	}

	return false
}
