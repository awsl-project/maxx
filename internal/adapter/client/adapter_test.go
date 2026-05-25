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

	req := httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(string(body)))
	if got := adapter.DetectClientType(req, body); got != domain.ClientTypeOpenAI {
		t.Fatalf("DetectClientType = %s, want %s", got, domain.ClientTypeOpenAI)
	}
	if got, ok := adapter.Match(req); !ok || got != domain.ClientTypeOpenAI {
		t.Fatalf("Match = (%s, %v), want (%s, true)", got, ok, domain.ClientTypeOpenAI)
	}

	// Model must be extractable from the body for routing/pricing.
	if got := adapter.ExtractModel(req, body, domain.ClientTypeOpenAI); got != "gpt-image-2" {
		t.Fatalf("ExtractModel = %q, want %q", got, "gpt-image-2")
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
