package mockserver

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestServer_DefaultOpenAI(t *testing.T) {
	srv := New()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(body, &result)
	if result["object"] != "chat.completion" {
		t.Errorf("expected chat.completion, got %v", result["object"])
	}
}

func TestServer_DefaultGemini(t *testing.T) {
	srv := New()
	defer srv.Close()

	// With provider path prefix
	resp, err := http.Post(srv.URL+"/p/1/v1beta/models/gemini-2.5-pro:generateContent", "application/json",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(body, &result)
	if _, ok := result["candidates"]; !ok {
		t.Error("expected candidates in Gemini response")
	}
}

func TestServer_ProviderURL(t *testing.T) {
	srv := New()
	defer srv.Close()

	url := srv.ProviderURL("42")
	if !strings.HasSuffix(url, "/p/42") {
		t.Errorf("expected /p/42 suffix, got %s", url)
	}
}

func TestServer_SetDirective_PerProvider(t *testing.T) {
	srv := New()
	defer srv.Close()

	// Provider 1: 429, Provider 2: 200
	session := srv.Set("", "1", MockDirective{Status: 429})
	srv.Set(session, "2", MockDirective{Status: 200})

	// Request as provider 1 → 429
	req1, _ := http.NewRequest("POST", srv.URL+"/p/1/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set(SessionHeader, session)
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != 429 {
		t.Fatalf("provider 1: expected 429, got %d", resp1.StatusCode)
	}

	// Request as provider 2 → 200
	req2, _ := http.NewRequest("POST", srv.URL+"/p/2/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set(SessionHeader, session)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("provider 2: expected 200, got %d", resp2.StatusCode)
	}
}

func TestServer_Wildcard(t *testing.T) {
	srv := New()
	defer srv.Close()

	session := srv.Set("", "*", MockDirective{Status: 503})

	req, _ := http.NewRequest("POST", srv.URL+"/p/99/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SessionHeader, session)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestServer_NoSession_Returns200(t *testing.T) {
	srv := New()
	defer srv.Close()

	srv.Set("", "1", MockDirective{Status: 500})

	// No session header → default 200
	resp, err := http.Post(srv.URL+"/p/1/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServer_SetViaHTTP(t *testing.T) {
	srv := New()
	defer srv.Close()

	setResp, err := http.Post(srv.URL+"/__mock/set", "application/json",
		strings.NewReader(`{"providerID":"5","directive":{"status":503}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer setResp.Body.Close()

	var setResult SetResponse
	json.NewDecoder(setResp.Body).Decode(&setResult)
	if setResult.Session == "" {
		t.Fatal("expected session")
	}

	req, _ := http.NewRequest("POST", srv.URL+"/p/5/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SessionHeader, setResult.Session)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestServer_Clear(t *testing.T) {
	srv := New()
	defer srv.Close()

	session := srv.Set("", "1", MockDirective{Status: 500})
	srv.Clear(session)

	req, _ := http.NewRequest("POST", srv.URL+"/p/1/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SessionHeader, session)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after clear, got %d", resp.StatusCode)
	}
}

func TestServer_LegacyMockHeader(t *testing.T) {
	srv := New()
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(MockHeader, `{"status":429,"headers":{"Retry-After":"5"}}`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 429 {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") != "5" {
		t.Errorf("expected Retry-After=5, got %q", resp.Header.Get("Retry-After"))
	}
}

func TestExtractProviderFromPath(t *testing.T) {
	tests := []struct {
		path       string
		wantID     string
		wantPath   string
	}{
		{"/p/1/v1/chat/completions", "1", "/v1/chat/completions"},
		{"/p/42/v1beta/models/gemini:generateContent", "42", "/v1beta/models/gemini:generateContent"},
		{"/p/3/v1/messages", "3", "/v1/messages"},
		{"/v1/chat/completions", "", "/v1/chat/completions"},  // no prefix
		{"/p/7", "7", "/"},                                     // no trailing path
	}
	for _, tt := range tests {
		id, path := extractProviderFromPath(tt.path)
		if id != tt.wantID || path != tt.wantPath {
			t.Errorf("extractProviderFromPath(%q) = (%q, %q), want (%q, %q)",
				tt.path, id, path, tt.wantID, tt.wantPath)
		}
	}
}
