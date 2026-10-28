package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

func TestLogWriterPreservesExistingBehavior(t *testing.T) {
	hub := &WebSocketHub{broadcast: make(chan WSMessage, 1)}
	var stdout bytes.Buffer
	path := filepath.Join(t.TempDir(), "maxx.log")
	writer := NewWebSocketLogWriter(hub, &stdout, path)
	defer writer.logFile.Close()
	message := "upstream rejected sk-proj-testtesttesttesttesttesttesttest\n"
	n, err := writer.Write([]byte(message))
	if err != nil || n != len(message) {
		t.Fatalf("io.Writer contract: n=%d err=%v", n, err)
	}
	file, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	event := <-hub.broadcast
	for _, sink := range []string{stdout.String(), string(file), event.Data.(string) + "\n"} {
		if sink != message {
			t.Fatalf("log unexpectedly changed: %s", sink)
		}
	}
}

func TestSelfServiceProjectPrivacyUpdate(t *testing.T) {
	repo := &selfServiceProjectRepo{projects: []*domain.Project{{ID: 1, TenantID: 1, Name: "protected", Slug: "protected", BlockSecrets: true}}}
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{projectRepo: repo})
	for _, tc := range []struct {
		body    string
		role    domain.UserRole
		status  int
		enabled bool
	}{
		{`{"name":"protected","slug":"protected"}`, domain.UserRoleAdmin, 200, true},
		{`{"name":"protected","slug":"protected","blockSecrets":false}`, domain.UserRoleMember, 403, true},
		{`{"name":"protected","slug":"protected","blockSecrets":false}`, domain.UserRoleAdmin, 200, false},
		{`{"name":"protected","slug":"protected","blockSecrets":true}`, domain.UserRoleAdmin, 200, true},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/projects/1", bytes.NewBufferString(tc.body))
		handler.ServeHTTP(rec, withSelfServiceContext(req, tc.role))
		if rec.Code != tc.status {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if repo.projects[0].BlockSecrets != tc.enabled {
			t.Fatal("project protection changed unexpectedly")
		}
		if rec.Code == 200 {
			var project domain.Project
			if err := json.Unmarshal(rec.Body.Bytes(), &project); err != nil || project.BlockSecrets != tc.enabled {
				t.Fatal("wrong project response")
			}
		}
	}
}

func TestSelfServicePrivacyEndpointOnlyChangesPolicy(t *testing.T) {
	repo := &selfServiceProjectRepo{projects: []*domain.Project{
		{ID: 1, TenantID: 1, Name: "original", Slug: "original", EnabledCustomRoutes: []domain.ClientType{domain.ClientTypeOpenAI}, BlockSecrets: true},
		{ID: 2, TenantID: 2, Name: "other", Slug: "other", BlockSecrets: true},
	}}
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{projectRepo: repo})
	for _, tc := range []struct {
		path, method, body string
		role               domain.UserRole
		status             int
		enabled            bool
	}{
		{"/projects/1/privacy", http.MethodPut, `{"blockSecrets":false}`, domain.UserRoleAdmin, 200, false},
		{"/projects/1/privacy", http.MethodPut, `{"blockSecrets":true}`, domain.UserRoleMember, 403, false},
		{"/projects/1/privacy", http.MethodPut, `{}`, domain.UserRoleAdmin, 400, false},
		{"/projects/1/privacy", http.MethodPut, `{"blockSecrets":null}`, domain.UserRoleAdmin, 400, false},
		{"/projects/1/privacy", http.MethodPut, `{"blockSecrets":true,"name":"changed"}`, domain.UserRoleAdmin, 400, false},
		{"/projects/1/privacy", http.MethodPut, `{"blockSecrets":true} {}`, domain.UserRoleAdmin, 400, false},
		{"/projects/2/privacy", http.MethodPut, `{"blockSecrets":false}`, domain.UserRoleAdmin, 404, false},
		{"/projects/1/privacy", http.MethodGet, ``, domain.UserRoleAdmin, 200, false},
		{"/projects/1/privacy", http.MethodPut, `{"blockSecrets":true}`, domain.UserRoleAdmin, 200, true},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		handler.ServeHTTP(rec, withSelfServiceContext(req, tc.role))
		if rec.Code != tc.status {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		project := repo.projects[0]
		if project.BlockSecrets != tc.enabled || project.Name != "original" || project.Slug != "original" || len(project.EnabledCustomRoutes) != 1 || !repo.projects[1].BlockSecrets {
			t.Fatal("privacy update changed unrelated settings or another tenant")
		}
	}
}

func TestFullPrivacyEndpointsRequireScopedAdmin(t *testing.T) {
	repo := &selfServiceProjectRepo{projects: []*domain.Project{{ID: 1, TenantID: 1, Name: "one", Slug: "one"}, {ID: 2, TenantID: 2, Name: "two", Slug: "two"}}}
	h := newSelfServiceHandlerForTests(selfServiceTestDeps{projectRepo: repo})
	for _, path := range []string{"/projects/1/privacy", "/projects/1/privacy/categories", "/projects/1/privacy/models", "/projects/1/privacy/records", "/projects/1/privacy/summary", "/projects/1/privacy/records/missing/value?placeholder=anything", "/projects/1/privacy/records/missing/sent"} {
		for _, role := range []domain.UserRole{domain.UserRoleMember, domain.UserRoleAdmin} {
			req := withSelfServiceContext(httptest.NewRequest(http.MethodGet, path, nil), role)
			if role == domain.UserRoleAdmin {
				req = req.WithContext(maxxctx.WithTenantID(req.Context(), 2))
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			want := 403
			if role == domain.UserRoleAdmin {
				want = 404
			}
			if w.Code != want {
				t.Fatalf("%s role=%s status=%d want=%d", path, role, w.Code, want)
			}
		}
	}
	req := withSelfServiceContext(httptest.NewRequest(http.MethodPost, "/projects/2/privacy/models/astrlink-guard/remove", nil), domain.UserRoleAdmin)
	req = req.WithContext(maxxctx.WithTenantID(req.Context(), 2))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("tenant admin could manage shared model: %d", w.Code)
	}
}
