package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWorkBuddySessionLimitsExpiryAndIsolation(t *testing.T) {
	h := NewWorkBuddyAIHandler()
	now := time.Now().UnixMilli()
	h.sessions["other"] = &workBuddySession{owner: "2/2", expires: now + 60000}
	h.sessions["expired"] = &workBuddySession{owner: "1/1", expires: now - 1}
	call := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		NoAuthMiddleware(h).ServeHTTP(w, httptest.NewRequest("POST", "/api/admin/workbuddy-ai"+path, strings.NewReader(body)))
		return w
	}
	if w := call("/auth/poll", `{"state":"other"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call("/auth/poll", `{"state":"expired"}`); !strings.Contains(w.Body.String(), "expired") {
		t.Fatal(w.Code, w.Body.String())
	}
	for i := 0; i < 8; i++ {
		h.sessions[fmt.Sprint(i)] = &workBuddySession{owner: "1/1", expires: now + 60000}
	}
	if w := call("/auth/start", "{}"); w.Code != 429 {
		t.Fatal(w.Code, w.Body.String())
	}
	h.sessions["waiting"] = &workBuddySession{owner: "1/1", expires: now + 60000, next: time.Now().Add(time.Hour)}
	if w := call("/auth/poll", `{"state":"waiting"}`); !strings.Contains(w.Body.String(), "pending") {
		t.Fatal(w.Body.String())
	}
	w := httptest.NewRecorder()
	NoAuthMiddleware(h).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/workbuddy-ai/models", nil))
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}
