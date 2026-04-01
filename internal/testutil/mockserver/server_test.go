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
	if result["model"] != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %v", result["model"])
	}
}

func TestServer_DefaultClaude(t *testing.T) {
	srv := New()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"claude-sonnet-4","messages":[{"role":"user","content":"hi"}]}`))
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
	if result["type"] != "message" {
		t.Errorf("expected type=message, got %v", result["type"])
	}
}

func TestServer_DefaultGemini(t *testing.T) {
	srv := New()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1beta/models/gemini-2.5-pro:generateContent", "application/json",
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

func TestServer_DefaultCodex(t *testing.T) {
	srv := New()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"gpt-4o","input":"hi"}`))
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
	if result["object"] != "response" {
		t.Errorf("expected object=response, got %v", result["object"])
	}
}

func TestServer_SetDirective_429(t *testing.T) {
	srv := New()
	defer srv.Close()

	session := srv.Set("", "my-api-key", MockDirective{
		Status:  429,
		Headers: map[string]string{"Retry-After": "5"},
	})

	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SessionHeader, session)
	req.Header.Set("Authorization", "Bearer my-api-key")

	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 429 {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") != "5" {
		t.Errorf("expected Retry-After=5, got %q", resp.Header.Get("Retry-After"))
	}
}

func TestServer_SetDirective_PerProvider(t *testing.T) {
	srv := New()
	defer srv.Close()

	session := srv.Set("", "provider-1-key", MockDirective{Status: 429})
	srv.Set(session, "provider-2-key", MockDirective{Status: 200})

	// Provider 1 → 429
	req1, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set(SessionHeader, session)
	req1.Header.Set("Authorization", "Bearer provider-1-key")
	resp1, _ := http.DefaultClient.Do(req1)
	defer resp1.Body.Close()
	if resp1.StatusCode != 429 {
		t.Fatalf("provider-1: expected 429, got %d", resp1.StatusCode)
	}

	// Provider 2 → 200
	req2, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set(SessionHeader, session)
	req2.Header.Set("Authorization", "Bearer provider-2-key")
	resp2, _ := http.DefaultClient.Do(req2)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("provider-2: expected 200, got %d", resp2.StatusCode)
	}
}

func TestServer_SetDirective_Wildcard(t *testing.T) {
	srv := New()
	defer srv.Close()

	session := srv.Set("", "*", MockDirective{Status: 503})

	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SessionHeader, session)
	req.Header.Set("Authorization", "Bearer any-key")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestServer_NoSession_Returns200(t *testing.T) {
	srv := New()
	defer srv.Close()

	srv.Set("", "some-key", MockDirective{Status: 500})

	// No session header → no match → 200
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer some-key")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 without session, got %d", resp.StatusCode)
	}
}

func TestServer_SetViaHTTP(t *testing.T) {
	srv := New()
	defer srv.Close()

	// Set via HTTP API
	setResp, _ := http.Post(srv.URL+"/__mock/set", "application/json",
		strings.NewReader(`{"apiKey":"test-key","directive":{"status":503}}`))
	defer setResp.Body.Close()

	var setResult SetResponse
	json.NewDecoder(setResp.Body).Decode(&setResult)
	if setResult.Session == "" {
		t.Fatal("expected session in response")
	}

	// Use the session
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SessionHeader, setResult.Session)
	req.Header.Set("Authorization", "Bearer test-key")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestServer_Clear(t *testing.T) {
	srv := New()
	defer srv.Close()

	session := srv.Set("", "key", MockDirective{Status: 500})
	srv.Clear(session)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SessionHeader, session)
	req.Header.Set("Authorization", "Bearer key")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after clear, got %d", resp.StatusCode)
	}
}

func TestServer_ExtractAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		header string
		value  string
		expect string
	}{
		{"openai", "Authorization", "Bearer sk-openai", "sk-openai"},
		{"claude", "x-api-key", "sk-claude", "sk-claude"},
		{"gemini", "x-goog-api-key", "gemini-key", "gemini-key"},
		{"none", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "http://localhost/test", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.value)
			}
			got := extractAPIKey(req)
			if got != tt.expect {
				t.Errorf("got %q, want %q", got, tt.expect)
			}
		})
	}
}
