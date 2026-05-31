package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/cfg"
	"github.com/awsl-project/maxx/internal/domain"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewFromContext(&cfg.Context{Name: "t", Server: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatalf("NewFromContext: %v", err)
	}
	return c, srv
}

func TestLogin(t *testing.T) {
	var seen struct {
		Path     string
		AuthHdr  string
		Username string
		Password string
	}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen.Path = r.URL.Path
		seen.AuthHdr = r.Header.Get("Authorization")
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		seen.Username = body["username"]
		seen.Password = body["password"]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"token":   "the-jwt",
			"user":    map[string]any{"id": 1, "username": "alice", "tenantID": 7, "tenantName": "Default", "role": "admin"},
		})
	})

	resp, err := c.Login("alice", "secret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if seen.Path != "/api/admin/auth/login" {
		t.Errorf("path = %q", seen.Path)
	}
	if seen.Username != "alice" || seen.Password != "secret" {
		t.Errorf("body = %+v", seen)
	}
	if resp.Token != "the-jwt" {
		t.Errorf("token = %q", resp.Token)
	}
}

func TestListProvidersSendsBearer(t *testing.T) {
	var gotAuth string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/admin/providers" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]*domain.Provider{{ID: 1, Type: "custom", Name: "test"}})
	})
	providers, err := c.ListProviders()
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if len(providers) != 1 || providers[0].Name != "test" {
		t.Errorf("providers = %+v", providers)
	}
}

func TestErrorBodyDecoded(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"name is required"}`))
	})
	_, err := c.CreateProvider(&domain.Provider{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("err = %v, want contains 'name is required'", err)
	}
}

func TestIsUnauthorizedDetects401(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	})
	_, err := c.ListProviders()
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsUnauthorized(err) {
		t.Errorf("IsUnauthorized returned false for %v", err)
	}
}

func TestJWTExpiry(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1700000000}`))
	tok := header + "." + payload + ".sig"
	got := JWTExpiry(tok)
	want := time.Unix(1700000000, 0)
	if !got.Equal(want) {
		t.Errorf("JWTExpiry = %v, want %v", got, want)
	}
}

func TestJWTExpiryHandlesGarbage(t *testing.T) {
	cases := []string{"", "abc", "a.b", "a.b.c"}
	for _, tc := range cases {
		if got := JWTExpiry(tc); !got.IsZero() {
			t.Errorf("JWTExpiry(%q) = %v, want zero", tc, got)
		}
	}
}

func TestNewFromContextRejectsBareHostPort(t *testing.T) {
	cases := map[string]string{
		"missing scheme":  "localhost:9880",
		"unknown scheme":  "ftp://example.com",
		"missing host":    "http://",
		"empty":           "",
	}
	for name, server := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewFromContext(&cfg.Context{Name: "t", Server: server}); err == nil {
				t.Errorf("NewFromContext(%q) returned no error", server)
			}
		})
	}
}

func TestNewFromContextAcceptsHTTPAndHTTPS(t *testing.T) {
	for _, server := range []string{"http://localhost:9880", "https://maxx.example.com"} {
		if _, err := NewFromContext(&cfg.Context{Name: "t", Server: server}); err != nil {
			t.Errorf("NewFromContext(%q) err = %v", server, err)
		}
	}
}

// TestInsecureSkipVerifyWarnsOncePerContext verifies the sync.Once gate by
// asking the warn function directly. It uses a unique context name so a
// re-run of the test in the same process behaves identically.
func TestInsecureSkipVerifyWarnsOncePerContext(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	name := fmt.Sprintf("test-once-%d", time.Now().UnixNano())
	for i := 0; i < 5; i++ {
		warnInsecure(name)
	}
	_ = w.Close()
	got, _ := io.ReadAll(r)
	n := strings.Count(string(got), "insecureSkipVerify=true")
	if n != 1 {
		t.Errorf("warnInsecure printed %d times for the same context, want 1", n)
	}
}

func TestExtractErrorMsgPicksKnownKeys(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"error", `{"error":"oops"}`, "oops"},
		{"message", `{"message":"bad input"}`, "bad input"},
		{"detail", `{"detail":"forbidden"}`, "forbidden"},
		// "error" wins over the others when more than one is present.
		{"error wins", `{"error":"first","message":"second"}`, "first"},
		// Empty "error" falls through to the next key.
		{"empty error falls through", `{"error":"","message":"fallback"}`, "fallback"},
		// Non-JSON body: returned trimmed verbatim.
		{"raw text", "plain text\n", "plain text"},
		// Genuinely empty body.
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractErrorMsg([]byte(c.body))
			if got != c.want {
				t.Errorf("extractErrorMsg(%q) = %q, want %q", c.body, got, c.want)
			}
		})
	}
}

func TestResponseTooLargeReturnsExplicitError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Stream more than the cap so the read hits the +1 byte and errors
		// rather than silently truncating to invalid JSON.
		oversized := make([]byte, maxResponseBytes+1024)
		for i := range oversized {
			oversized[i] = 'a'
		}
		_, _ = w.Write(oversized)
	})
	_, err := c.ListProviders()
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("err = %v, want it to mention exceeding the cap", err)
	}
}

func TestPartialRouteUpdatePatch(t *testing.T) {
	var bodyBytes []byte
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(&domain.Route{ID: 42, Weight: 3})
	})
	_, err := c.UpdateRoute(42, map[string]any{"weight": 3})
	if err != nil {
		t.Fatalf("UpdateRoute: %v", err)
	}
	if !strings.Contains(string(bodyBytes), `"weight":3`) {
		t.Errorf("body did not include weight: %s", bodyBytes)
	}
}
