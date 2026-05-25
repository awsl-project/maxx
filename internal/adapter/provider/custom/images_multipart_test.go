package custom

import (
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

// TestUpdateModelInBody_FailsOnMultipart guards the contract behind the
// isMultipartForm skip in Execute: updateModelInBody JSON-decodes the body, so a
// multipart/form-data upload (OpenAI images/edits) genuinely fails here. Execute
// must skip the rewrite for multipart, or edits requests 502 with
// "failed to update model in body".
func TestUpdateModelInBody_FailsOnMultipart(t *testing.T) {
	multipartBody := []byte("--abc\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\ngpt-image-2\r\n--abc--\r\n")
	if _, err := updateModelInBody(multipartBody, "gpt-image-2", domain.ClientTypeOpenAI); err == nil {
		t.Fatal("expected updateModelInBody to fail on multipart body (this is why Execute must skip it)")
	}
}
