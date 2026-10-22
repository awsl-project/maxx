package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/awsl-project/maxx/internal/adapter/provider/workbuddyai"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkBuddyProtectedBrowserContract(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/plugin/auth/state":
			fmt.Fprint(w, `{"data":{"state":"remote","authUrl":"https://example.com/login"}}`)
		case "/v2/plugin/auth/token":
			fmt.Fprint(w, `{"data":{"accessToken":"dummy"}}`)
		case "/v2/plugin/login/account":
			fmt.Fprint(w, `{"data":{"uid":"u","type":"personal"}}`)
		case "/v3/config":
			fmt.Fprint(w, `{"data":{"agents":[{"name":"cli","models":["fast"]}]}}`)
		}
	}))
	defer s.Close()
	h := NewWorkBuddyAIHandler(workbuddyai.NewClient(s.URL, s.Client()))
	mux := http.NewServeMux()
	RegisterWorkBuddyAIRoutes(mux, NoAuthMiddleware, h)
	post := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/admin/workbuddy-ai"+path, bytes.NewBufferString(body)))
		return w
	}
	w := post("/auth/start", "{}")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var start workbuddyai.StartResult
	json.Unmarshal(w.Body.Bytes(), &start)
	if start.State == "remote" || start.State == "" {
		t.Fatal("state not opaque")
	}
	w = post("/auth/poll", fmt.Sprintf(`{"state":%q}`, start.State))
	var poll struct {
		Status string
		Config workbuddyai.Config
	}
	json.Unmarshal(w.Body.Bytes(), &poll)
	if w.Code != 200 || poll.Status != "authorized" || poll.Config.UID != "u" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = post("/models", `{"accessToken":"dummy"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if post("/auth/poll", `{"state":"unknown"}`).Code != 400 {
		t.Fatal("unknown state accepted")
	}
	raw := httptest.NewRecorder()
	h.ServeHTTP(raw, httptest.NewRequest("POST", "/api/admin/workbuddy-ai/auth/start", nil))
	if raw.Code != 403 {
		t.Fatal("missing admin accepted")
	}
	protected := http.NewServeMux()
	RegisterWorkBuddyAIRoutes(protected, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unauthorized", 401) })
	}, h)
	raw = httptest.NewRecorder()
	protected.ServeHTTP(raw, httptest.NewRequest("POST", "/api/admin/workbuddy-ai/auth/start", nil))
	if raw.Code != 401 {
		t.Fatal("wrapper bypassed")
	}
}
