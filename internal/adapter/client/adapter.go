package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/Bowl42/maxx-next/internal/domain"
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
	case strings.HasPrefix(path, "/v1/chat/completions"):
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
	// For Gemini, try URL first
	if clientType == domain.ClientTypeGemini {
		path := req.URL.Path
		if matches := geminiModelPattern.FindStringSubmatch(path); len(matches) > 1 {
			return matches[1]
		}
		if matches := geminiInternalPattern.FindStringSubmatch(path); len(matches) > 1 {
			return matches[1]
		}
	}

	// Try body
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	if model, ok := data["model"].(string); ok {
		return model
	}

	return ""
}

func (a *Adapter) extractSessionID(req *http.Request, clientType domain.ClientType, body []byte) string {
	// 1. Try metadata.session_id (Claude)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err == nil {
		if metadata, ok := data["metadata"].(map[string]interface{}); ok {
			if sid, ok := metadata["session_id"].(string); ok && sid != "" {
				return sid
			}
		}
	}

	// 2. Try Header X-Session-Id
	if sid := req.Header.Get("X-Session-Id"); sid != "" {
		return sid
	}

	// 3. Generate deterministic session ID from request characteristics
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
	case strings.HasPrefix(path, "/responses"):
		return domain.ClientTypeCodex
	case strings.HasPrefix(path, "/v1/chat/completions"):
		return domain.ClientTypeOpenAI
	case strings.HasPrefix(path, "/v1beta/models/"):
		return domain.ClientTypeGemini
	case strings.HasPrefix(path, "/v1internal/models/"):
		return domain.ClientTypeGemini
	}

	// Second layer: body detection (fallback)
	return a.detectFromBodyBytes(body)
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

	// Try body
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	if model, ok := data["model"].(string); ok {
		return model
	}

	return ""
}

// ExtractSessionID extracts the session ID from request
func (a *Adapter) ExtractSessionID(req *http.Request, body []byte, clientType domain.ClientType) string {
	return a.extractSessionID(req, clientType, body)
}

// IsStreamRequest checks if the request is for streaming
func (a *Adapter) IsStreamRequest(body []byte) bool {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return false
	}

	if stream, ok := data["stream"].(bool); ok {
		return stream
	}

	return false
}
