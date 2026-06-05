package custom

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestIsMultipartForm(t *testing.T) {
	cases := []struct {
		ct   string
		want bool
	}{
		{"multipart/form-data; boundary=abc", true},
		{"Multipart/Form-Data; boundary=xyz", true},
		{"application/json", false},
		{"text/plain", false},
		{"", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/v1/images/edits", nil)
		if c.ct != "" {
			req.Header.Set("Content-Type", c.ct)
		}
		if got := isMultipartForm(req); got != c.want {
			t.Errorf("isMultipartForm(%q) = %v, want %v", c.ct, got, c.want)
		}
	}
	if isMultipartForm(nil) {
		t.Error("isMultipartForm(nil) = true, want false")
	}
}

// TestUpdateModelInBody_FailsOnMultipart documents why Execute can't use the
// JSON rewrite path for multipart bodies: updateModelInBody JSON-decodes the
// body, so a multipart/form-data upload (OpenAI images/edits) genuinely fails
// here. That's why Execute routes multipart through updateModelInMultipartForm
// instead.
func TestUpdateModelInBody_FailsOnMultipart(t *testing.T) {
	multipartBody := []byte("--abc\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\ngpt-image-2\r\n--abc--\r\n")
	if _, err := updateModelInBody(multipartBody, "gpt-image-2", domain.ClientTypeOpenAI); err == nil {
		t.Fatal("expected updateModelInBody to fail on multipart body (this is why Execute must skip it)")
	}
}

// buildEditsMultipart builds a multipart/form-data body mimicking an OpenAI
// images/edits request, plus the matching Content-Type (with boundary).
func buildEditsMultipart(t *testing.T, model string, withModel bool) (body []byte, contentType string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if withModel {
		if err := mw.WriteField("model", model); err != nil {
			t.Fatalf("WriteField model: %v", err)
		}
	}
	if err := mw.WriteField("prompt", "make it blue"); err != nil {
		t.Fatalf("WriteField prompt: %v", err)
	}
	fw, err := mw.CreateFormFile("image", "input.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write([]byte("\x89PNG\r\n\x1a\nfake-image-bytes")); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return buf.Bytes(), mw.FormDataContentType()
}

// parseField returns the value of a single form field from a multipart body.
func parseField(t *testing.T, body []byte, boundary, name string) (string, bool) {
	t.Helper()
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return "", false
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		if part.FormName() == name {
			b, _ := io.ReadAll(part)
			_ = part.Close()
			return string(b), true
		}
		_ = part.Close()
	}
}

// TestUpdateModelInMultipartForm_RewritesModel verifies that the multipart
// rewrite applies model mapping to the "model" field, preserves the uploaded
// image and other fields untouched, and reuses the original boundary so the
// inbound Content-Type header stays valid.
func TestUpdateModelInMultipartForm_RewritesModel(t *testing.T) {
	body, contentType := buildEditsMultipart(t, "gpt-image-2", true)
	req := httptest.NewRequest("POST", "/v1/images/edits", nil)
	req.Header.Set("Content-Type", contentType)

	out, err := updateModelInMultipartForm(body, req, "provider-image-model")
	if err != nil {
		t.Fatalf("updateModelInMultipartForm: %v", err)
	}

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType: %v", err)
	}
	boundary := params["boundary"]

	if got, ok := parseField(t, out, boundary, "model"); !ok || got != "provider-image-model" {
		t.Fatalf("model field = %q (ok=%v), want %q", got, ok, "provider-image-model")
	}
	if got, ok := parseField(t, out, boundary, "prompt"); !ok || got != "make it blue" {
		t.Fatalf("prompt field = %q (ok=%v), want %q", got, ok, "make it blue")
	}
	if got, ok := parseField(t, out, boundary, "image"); !ok || got != "\x89PNG\r\n\x1a\nfake-image-bytes" {
		t.Fatalf("image part not preserved: got %q (ok=%v)", got, ok)
	}
}

// TestUpdateModelInMultipartForm_AddsModelWhenAbsent ensures a "model" field is
// appended when the original body omitted it, so mapping still takes effect.
func TestUpdateModelInMultipartForm_AddsModelWhenAbsent(t *testing.T) {
	body, contentType := buildEditsMultipart(t, "", false)
	req := httptest.NewRequest("POST", "/v1/images/edits", nil)
	req.Header.Set("Content-Type", contentType)

	out, err := updateModelInMultipartForm(body, req, "provider-image-model")
	if err != nil {
		t.Fatalf("updateModelInMultipartForm: %v", err)
	}

	_, params, _ := mime.ParseMediaType(contentType)
	if got, ok := parseField(t, out, params["boundary"], "model"); !ok || got != "provider-image-model" {
		t.Fatalf("model field = %q (ok=%v), want appended %q", got, ok, "provider-image-model")
	}
}

// TestUpdateModelInMultipartForm_NonMultipart returns an error for a body whose
// Content-Type lacks a boundary, so Execute surfaces a clear 502 rather than
// forwarding a corrupted body.
func TestUpdateModelInMultipartForm_NonMultipart(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/images/edits", nil)
	req.Header.Set("Content-Type", "application/json")
	if _, err := updateModelInMultipartForm([]byte("{}"), req, "x"); err == nil {
		t.Fatal("expected error for body without multipart boundary")
	}
}
