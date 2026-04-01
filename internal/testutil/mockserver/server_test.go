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
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
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

func TestServer_MockHeader429(t *testing.T) {
	srv := New()
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
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

	// Should have protocol-appropriate error body
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(body, &result)
	if _, ok := result["error"]; !ok {
		t.Errorf("expected error in response body: %s", body)
	}
}

func TestServer_MockHeaderCustomBody(t *testing.T) {
	srv := New()
	defer srv.Close()

	customBody := `{"custom":"error","detail":"test"}`
	mockDirective := `{"status":503,"body":` + customBody + `}`

	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(MockHeader, mockDirective)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(body, &result)
	if result["custom"] != "error" {
		t.Errorf("expected custom body, got: %s", body)
	}
}

func TestServer_MockHeaderStream(t *testing.T) {
	srv := New()
	defer srv.Close()

	mockDirective := `{"stream":{"chunks":[{"data":{"text":"hello"}},{"data":{"text":"world"}}]}}`

	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(MockHeader, mockDirective)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello") || !strings.Contains(string(body), "world") {
		t.Errorf("expected stream chunks: %s", body)
	}
}

func TestServer_InvalidMockHeader(t *testing.T) {
	srv := New()
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(MockHeader, "not valid json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for invalid mock header, got %d", resp.StatusCode)
	}
}
