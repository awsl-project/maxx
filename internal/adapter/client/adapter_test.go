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
