package workbuddyai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRenewalScopeIsolation(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprintf(w, `{"data":{"accessToken":%q,"refreshToken":"rotated"}}`, r.Header.Get("X-Domain")+"/"+r.Header.Get("X-Tenant-Id"))
	}))
	defer s.Close()
	c := NewClient(s.URL, s.Client())
	for _, a := range []Config{
		{RefreshToken: "dummy", Domain: "example.com", UID: "one", EnterpriseID: "team-one"},
		{RefreshToken: "dummy", Domain: "example.com", UID: "one", EnterpriseID: "team-two"},
		{RefreshToken: "dummy", Domain: "other.example.com", UID: "one", EnterpriseID: "team-one"},
		{RefreshToken: "dummy", Domain: "example.com", UID: "two", EnterpriseID: "team-one"},
	} {
		for i := 0; i < 2; i++ {
			n, err := c.Refresh(context.Background(), a)
			if err != nil || n.AccessToken != a.Domain+"/"+a.EnterpriseID || n.UID != a.UID || n.EnterpriseID != a.EnterpriseID {
				t.Fatalf("scope isolation failed: %v", err)
			}
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("got %d refreshes, want four isolated scopes", calls.Load())
	}
}

func TestChatErrorDoesNotEchoUpstreamSecrets(t *testing.T) {
	for _, payload := range []string{
		`{"code":10085,"msg":"Bearer dummy-secret"}`,
		`{"code":"dummy-secret","message":"dummy-secret"}`,
		`{"error":{"message":"dummy-secret"}}`,
	} {
		err := readUpstreamError(strings.NewReader(payload), 403)
		if err == nil || strings.Contains(err.Error(), "dummy-secret") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
}

func TestChatSecretEchoHTTPAndSSE(t *testing.T) {
	for _, status := range []int{200, 403} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(status)
			if status == 200 {
				fmt.Fprint(w, "data: ")
			}
			fmt.Fprint(w, "{\"code\":10085,\"msg\":\"Bearer dummy-secret\"}\n\n")
		}))
		w, err := executeTest(t, server, "{\"messages\":[],\"stream\":true}")
		server.Close()
		if err == nil || strings.Contains(err.Error(), "dummy-secret") || !strings.Contains(err.Error(), "10085") || w.Body.Len() != 0 {
			t.Fatalf("unsafe response: %v", err)
		}
	}
}

func TestCompletionTokenCapSpelling(t *testing.T) {
	for _, tc := range []struct {
		body string
		want string
	}{
		{"{\"messages\":[],\"max_completion_tokens\":17}", "17"},
		{"{\"messages\":[],\"max_completion_tokens\":17,\"max_tokens\":9}", "9"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				t.Error(err)
			}
			if string(b["max_tokens"]) != tc.want || b["max_completion_tokens"] != nil {
				t.Error("completion cap not preserved")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, textFrames)
		}))
		_, err := executeTest(t, server, tc.body)
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
