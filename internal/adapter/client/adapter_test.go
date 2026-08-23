package client

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestDetectClientTypePrefersClaudeUserAgent(t *testing.T) {
	adapter := NewAdapter()
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)

	req := httptest.NewRequest("POST", "/unknown", strings.NewReader(string(body)))
	req.Header.Set("User-Agent", "claude-cli/2.0")
	if got := adapter.DetectClientType(req, body); got != domain.ClientTypeClaude {
		t.Fatalf("client type = %s, want %s", got, domain.ClientTypeClaude)
	}

	req = httptest.NewRequest("POST", "/unknown", strings.NewReader(string(body)))
	req.Header.Set("User-Agent", "curl/7.0")
	if got := adapter.DetectClientType(req, body); got != domain.ClientTypeOpenAI {
		t.Fatalf("client type = %s, want %s", got, domain.ClientTypeOpenAI)
	}

	req = httptest.NewRequest("POST", "/unknown", strings.NewReader(string(body)))
	req.Header.Set("User-Agent", " Claude-cli/2.0")
	if got := adapter.DetectClientType(req, body); got != domain.ClientTypeOpenAI {
		t.Fatalf("client type = %s, want %s", got, domain.ClientTypeOpenAI)
	}
}

func TestDetectClientTypeRecognizesImagesPath(t *testing.T) {
	adapter := NewAdapter()
	// Images generation body has neither messages/input/contents — only path can classify it.
	body := []byte(`{"model":"gpt-image-2","prompt":"a cat","n":1,"size":"1024x1024"}`)

	for _, path := range []string{"/v1/images/generations", "/images/generations"} {
		req := httptest.NewRequest("POST", path, strings.NewReader(string(body)))
		if got := adapter.DetectClientType(req, body); got != domain.ClientTypeOpenAI {
			t.Fatalf("DetectClientType(%s) = %s, want %s", path, got, domain.ClientTypeOpenAI)
		}
		if got, ok := adapter.Match(req); !ok || got != domain.ClientTypeOpenAI {
			t.Fatalf("Match(%s) = (%s, %v), want (%s, true)", path, got, ok, domain.ClientTypeOpenAI)
		}

		// Model must be extractable from the body for routing/pricing.
		if got := adapter.ExtractModel(req, body, domain.ClientTypeOpenAI); got != "gpt-image-2" {
			t.Fatalf("ExtractModel(%s) = %q, want %q", path, got, "gpt-image-2")
		}
	}

	chatBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	chatReq := httptest.NewRequest("POST", "/chat/completions", strings.NewReader(string(chatBody)))
	if got := adapter.DetectClientType(chatReq, chatBody); got != domain.ClientTypeOpenAI {
		t.Fatalf("DetectClientType(/chat/completions) = %s, want %s", got, domain.ClientTypeOpenAI)
	}
	if got, ok := adapter.Match(chatReq); !ok || got != domain.ClientTypeOpenAI {
		t.Fatalf("Match(/chat/completions) = (%s, %v), want (%s, true)", got, ok, domain.ClientTypeOpenAI)
	}
}

// OpenRouter's unified image endpoint is the BARE path /v1/images (no
// /generations suffix). Its {model,prompt} body has no messages/input/contents,
// so only the path can classify it — the bare path must resolve to openai
// instead of falling through to a 400.
func TestDetectClientTypeRecognizesBareImagesPath(t *testing.T) {
	adapter := NewAdapter()
	body := []byte(`{"model":"google/gemini-2.5-flash-image","prompt":"a cat"}`)

	for _, path := range []string{"/v1/images", "/images"} {
		req := httptest.NewRequest("POST", path, strings.NewReader(string(body)))
		if got := adapter.DetectClientType(req, body); got != domain.ClientTypeOpenAI {
			t.Fatalf("DetectClientType(%s) = %s, want %s", path, got, domain.ClientTypeOpenAI)
		}
		if got, ok := adapter.Match(req); !ok || got != domain.ClientTypeOpenAI {
			t.Fatalf("Match(%s) = (%s, %v), want (%s, true)", path, got, ok, domain.ClientTypeOpenAI)
		}
	}
}

func TestDetectClientTypeRecognizesVideoGenerationsPath(t *testing.T) {
	adapter := NewAdapter()

	// Submit: POST with a JSON body carrying the model.
	submitBody := []byte(`{"model":"doubao-seedance-2-0-260128","prompt":"a cat runs"}`)
	for _, path := range []string{"/v1/video/generations", "/video/generations"} {
		req := httptest.NewRequest("POST", path, strings.NewReader(string(submitBody)))
		if got := adapter.DetectClientType(req, submitBody); got != domain.ClientTypeVideo {
			t.Fatalf("DetectClientType(POST %s) = %s, want %s", path, got, domain.ClientTypeVideo)
		}
		if got, ok := adapter.Match(req); !ok || got != domain.ClientTypeVideo {
			t.Fatalf("Match(POST %s) = (%s, %v), want (%s, true)", path, got, ok, domain.ClientTypeVideo)
		}
		if got := adapter.ExtractModel(req, submitBody, domain.ClientTypeVideo); got != "doubao-seedance-2-0-260128" {
			t.Fatalf("ExtractModel(%s) = %q, want the seedance model", path, got)
		}
	}

	// Poll: GET /{task_id} with no body — classified by path, model is empty.
	for _, path := range []string{"/v1/video/generations/task_abc123", "/video/generations/task_abc123"} {
		req := httptest.NewRequest("GET", path, nil)
		if got := adapter.DetectClientType(req, nil); got != domain.ClientTypeVideo {
			t.Fatalf("DetectClientType(GET %s) = %s, want %s", path, got, domain.ClientTypeVideo)
		}
		if got, ok := adapter.Match(req); !ok || got != domain.ClientTypeVideo {
			t.Fatalf("Match(GET %s) = (%s, %v), want (%s, true)", path, got, ok, domain.ClientTypeVideo)
		}
		if got := adapter.ExtractModel(req, nil, domain.ClientTypeVideo); got != "" {
			t.Fatalf("ExtractModel(poll %s) = %q, want empty", path, got)
		}
	}
}

// Only the poll (collection path + a non-empty task id) may be a GET; the bare
// submit endpoints stay POST-only, so the method gate must not treat them as polls.
func TestIsVideoPollPath(t *testing.T) {
	polls := []string{"/v1/video/generations/task_abc123", "/video/generations/task_abc123"}
	for _, p := range polls {
		if !IsVideoPollPath(p) {
			t.Fatalf("IsVideoPollPath(%s) = false, want true", p)
		}
	}
	notPolls := []string{
		"/v1/video/generations", "/video/generations",
		"/v1/video/generations/", "/video/generations/", // trailing slash, no task id
		"/v1/chat/completions",
	}
	for _, p := range notPolls {
		if IsVideoPollPath(p) {
			t.Fatalf("IsVideoPollPath(%s) = true, want false", p)
		}
	}
}

func TestImagesEdits_MultipartModelExtraction(t *testing.T) {
	adapter := NewAdapter()

	// Build a multipart/form-data body like OpenAI images/edits: a model field
	// plus an "image" file part. Order matters — put the image first to prove we
	// still find "model" after skipping a (here small) upload.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("image", "in.png")
	fw.Write([]byte("\x89PNG\r\n\x1a\n fake image bytes"))
	mw.WriteField("model", "gpt-image-2")
	mw.WriteField("prompt", "make it blue")
	mw.Close()
	body := buf.Bytes()

	req := httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", mw.FormDataContentType())

	if got := adapter.DetectClientType(req, body); got != domain.ClientTypeOpenAI {
		t.Fatalf("DetectClientType = %s, want %s", got, domain.ClientTypeOpenAI)
	}
	// Model must come from the multipart form (JSON unmarshal of this body fails).
	if got := adapter.ExtractModel(req, body, domain.ClientTypeOpenAI); got != "gpt-image-2" {
		t.Fatalf("ExtractModel = %q, want %q", got, "gpt-image-2")
	}
}

func TestDetectClientTypeRecognizesV1ResponsesPath(t *testing.T) {
	adapter := NewAdapter()
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(string(body)))
	if got := adapter.DetectClientType(req, body); got != domain.ClientTypeCodex {
		t.Fatalf("client type = %s, want %s", got, domain.ClientTypeCodex)
	}

	req = httptest.NewRequest("POST", "/v1/responses/create", strings.NewReader(string(body)))
	if got := adapter.DetectClientType(req, body); got != domain.ClientTypeCodex {
		t.Fatalf("client type = %s, want %s", got, domain.ClientTypeCodex)
	}
}

func TestExtractSessionIDFromJSONFields(t *testing.T) {
	adapter := NewAdapter()
	tests := []struct {
		name       string
		clientType domain.ClientType
		body       string
		want       string
	}{
		{
			name:       "codex previous response",
			clientType: domain.ClientTypeCodex,
			body:       `{"previous_response_id":"resp_123","prompt_cache_key":"cache_ignored"}`,
			want:       "resp_123",
		},
		{
			name:       "codex prompt cache",
			clientType: domain.ClientTypeCodex,
			body:       `{"prompt_cache_key":"cache_123"}`,
			want:       "cache_123",
		},
		{
			name:       "metadata session",
			clientType: domain.ClientTypeClaude,
			body:       `{"metadata":{"session_id":"session_123"}}`,
			want:       "session_123",
		},
		{
			name:       "claude user session suffix",
			clientType: domain.ClientTypeClaude,
			body:       `{"metadata":{"user_id":"user_hash_account__session_uuid-123"}}`,
			want:       "uuid-123",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(test.body)
			req := httptest.NewRequest("POST", "/unknown", bytes.NewReader(body))
			if got := adapter.ExtractSessionID(req, body, test.clientType); got != test.want {
				t.Fatalf("session ID = %q, want %q", got, test.want)
			}
		})
	}
}

func TestIsStreamRequestReadsBooleanOnly(t *testing.T) {
	adapter := NewAdapter()
	req := httptest.NewRequest("POST", "/v1/responses", nil)
	if !adapter.IsStreamRequest(req, []byte(`{"stream":true,"input":[]}`)) {
		t.Fatal("stream=true was not detected")
	}
	for _, body := range []string{`{"stream":false}`, `{"stream":"true"}`, `{}`} {
		if adapter.IsStreamRequest(req, []byte(body)) {
			t.Fatalf("unexpected stream detection for %s", body)
		}
	}
}
