package handler

import (
	"bytes"
	"github.com/awsl-project/maxx/internal/privacy"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

type memoryGlobalPrivacySettings struct{ value string }

func (m *memoryGlobalPrivacySettings) Get(string) (string, error)       { return m.value, nil }
func (m *memoryGlobalPrivacySettings) Set(_ string, value string) error { m.value = value; return nil }

func TestGlobalPrivacyEndpointAndAccess(t *testing.T) {
	svc := newSelfServiceHandlerForTests(selfServiceTestDeps{}).svc
	svc.SetPrivacyService(privacy.NewService(&memoryGlobalPrivacySettings{}))
	handler := NewAdminHandler(svc, nil, "")
	for _, tc := range []struct {
		path, method, body string
		role               domain.UserRole
		tenant             uint64
		status             int
	}{
		{"/admin/privacy", "GET", "", domain.UserRoleAdmin, 1, 200},
		{"/admin/privacy", "PUT", `{"config":{"enabled":true,"secrets":true,"mode":"mask","model":{"enabled":false,"name":"astrlink-guard"}}}`, domain.UserRoleAdmin, 1, 200},
		{"/admin/privacy/preview", "POST", `{"config":{"enabled":true,"secrets":true},"text":"sk-proj-testtesttesttesttesttesttesttest"}`, domain.UserRoleAdmin, 1, 200},
		{"/admin/privacy", "PUT", `{"blockSecrets":false}`, domain.UserRoleAdmin, 1, 400},
		{"/admin/privacy", "PUT", `{"config":{}} {}`, domain.UserRoleAdmin, 1, 400},
		{"/admin/privacy/models", "GET", "", domain.UserRoleAdmin, 1, 200},
		{"/admin/privacy/records", "GET", "", domain.UserRoleAdmin, 1, 200},
		{"/admin/privacy/summary", "GET", "", domain.UserRoleAdmin, 1, 200},
		{"/admin/settings/privacy_policy", "GET", "", domain.UserRoleAdmin, 1, 403},
		{"/admin/settings/privacy_policy", "PUT", `{"value":"{}"}`, domain.UserRoleAdmin, 2, 403},
		{"/admin/settings/privacy_policy", "DELETE", "", domain.UserRoleAdmin, 2, 403},
		{"/admin/privacy", "GET", "", domain.UserRoleMember, 1, 403},
		{"/admin/privacy", "PUT", `{"config":{}}`, domain.UserRoleAdmin, 2, 403},
		{"/admin/privacy/records", "GET", "", domain.UserRoleAdmin, 2, 403},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req = withSelfServiceContext(req, tc.role)
		req = req.WithContext(maxxctx.WithTenantID(req.Context(), tc.tenant))
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	cfg, err := svc.Privacy().Configuration()
	if err != nil || !cfg.Enabled {
		t.Fatal("global update did not persist")
	}
}
func TestProjectPrivacyRoutesAreRemoved(t *testing.T) {
	h := newSelfServiceHandlerForTests(selfServiceTestDeps{})
	for _, path := range []string{"/projects/1/privacy", "/projects/1/privacy/models", "/projects/1/privacy/records"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, withSelfServiceContext(httptest.NewRequest("GET", path, nil), domain.UserRoleAdmin))
		if rec.Code != 404 {
			t.Fatalf("project privacy route remains: %s: %d", path, rec.Code)
		}
	}
}

func TestPrivacyAdministratorCanRepairInvalidPolicy(t *testing.T) {
	svc := newSelfServiceHandlerForTests(selfServiceTestDeps{}).svc
	store := &memoryGlobalPrivacySettings{value: "{invalid"}
	svc.SetPrivacyService(privacy.NewService(store))
	h := NewAdminHandler(svc, nil, "")
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/admin/privacy", "", 500},
		{"GET", "/admin/privacy/categories", "", 200},
		{"GET", "/admin/privacy/models", "", 200},
		{"GET", "/admin/privacy/records", "", 200},
		{"GET", "/admin/privacy/summary", "", 200},
		{"GET", "/admin/privacy/summary/unexpected", "", 404},
		{"PUT", "/admin/privacy", `{"config":{"enabled":true,"secrets":true,"mode":"mask"}}`, 200},
		{"GET", "/admin/privacy", "", 200},
	} {
		req := withSelfServiceContext(httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)), domain.UserRoleAdmin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s %s: got %d, want %d: %s", tc.method, tc.path, rec.Code, tc.status, rec.Body.String())
		}
	}
	cfg, err := svc.Privacy().Configuration()
	if err != nil || !cfg.Enabled || !cfg.Secrets {
		t.Fatal("repair did not persist the replacement policy", err)
	}
}
