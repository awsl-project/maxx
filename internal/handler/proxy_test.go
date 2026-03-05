package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "bad request")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var payload map[string]map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload["error"]["message"] != "bad request" {
		t.Fatalf("payload = %v, want error message", payload)
	}
}

func TestIsCodexCompactionEligiblePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/responses", want: true},
		{path: "/v1/responses", want: true},
		{path: "/v1/chat/completions", want: false},
		{path: "/messages", want: false},
	}

	for _, tt := range tests {
		got := isCodexCompactionEligiblePath(tt.path)
		if got != tt.want {
			t.Fatalf("path %q eligible = %v, want %v", tt.path, got, tt.want)
		}
	}
}
