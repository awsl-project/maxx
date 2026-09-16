package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeGrokOAuthClient struct {
	calls []string
}

func (f *fakeGrokOAuthClient) Do(req *http.Request) (*http.Response, error) {
	f.calls = append(f.calls, req.Method+" "+req.URL.String())
	body := `{} `
	status := http.StatusOK
	switch req.URL.String() {
	case "https://auth.x.ai/.well-known/openid-configuration":
		body = `{"device_authorization_endpoint":"https://auth.x.ai/device","token_endpoint":"https://auth.x.ai/token"}`
	case "https://auth.x.ai/device":
		raw, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(raw), "client_id=") || !strings.Contains(string(raw), "scope=") {
			status = http.StatusBadRequest
			body = `{"error":"bad_form"}`
		} else {
			body = `{"device_code":"device-1","user_code":"ABCD-EFGH","verification_uri":"https://x.ai/device","verification_uri_complete":"https://x.ai/device?user_code=ABCD-EFGH","expires_in":600,"interval":5}`
		}
	case "https://auth.x.ai/token":
		body = `{"access_token":"access-token","refresh_token":"refresh-token","id_token":"` + testGrokIDToken() + `","token_type":"Bearer","expires_in":3600}`
	default:
		status = http.StatusNotFound
		body = `{"error":"not_found"}`
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func testGrokIDToken() string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"grok@example.com","sub":"subject-1"}`))
	return "header." + payload + ".sig"
}

func TestGrokOAuthStartAndPollReturnsProviderConfig(t *testing.T) {
	client := &fakeGrokOAuthClient{}
	h := NewGrokHandler()
	h.httpClient = client

	start, err := h.StartOAuth(context.Background())
	if err != nil {
		t.Fatalf("StartOAuth error: %v", err)
	}
	if start.SessionID == "" || start.UserCode != "ABCD-EFGH" || start.VerificationURIComplete == "" {
		t.Fatalf("start = %#v, want session and verification fields", start)
	}

	poll, err := h.PollOAuth(context.Background(), start.SessionID)
	if err != nil {
		t.Fatalf("PollOAuth error: %v", err)
	}
	if poll.Status != "authorized" || poll.Config == nil {
		t.Fatalf("poll = %#v, want authorized config", poll)
	}
	if poll.Config.Type != "xai" || poll.Config.AuthKind != "oauth" {
		t.Fatalf("config = %#v, want xai oauth", poll.Config)
	}
	if poll.Config.Email != "grok@example.com" || poll.Config.Sub != "subject-1" {
		t.Fatalf("identity = %q/%q, want parsed id token identity", poll.Config.Email, poll.Config.Sub)
	}
	if poll.Config.AccessToken != "access-token" || poll.Config.RefreshToken != "refresh-token" {
		t.Fatalf("tokens not mapped: %#v", poll.Config)
	}
	if poll.Config.TokenEndpoint != "https://auth.x.ai/token" || poll.Config.BaseURL != grokOAuthDefaultBaseURL {
		t.Fatalf("endpoints not mapped: %#v", poll.Config)
	}
}

func TestValidateGrokOAuthEndpointRejectsNonXAIHosts(t *testing.T) {
	if _, err := validateGrokOAuthEndpoint("https://evil.example/device", "device_authorization_endpoint"); err == nil {
		t.Fatal("expected non-x.ai endpoint to be rejected")
	}
	if _, err := validateGrokOAuthEndpoint("http://auth.x.ai/device", "device_authorization_endpoint"); err == nil {
		t.Fatal("expected non-https endpoint to be rejected")
	}
	if got, err := validateGrokOAuthEndpoint("https://auth.x.ai/device", "device_authorization_endpoint"); err != nil || got == "" {
		t.Fatalf("valid x.ai endpoint rejected: got=%q err=%v", got, err)
	}
}
