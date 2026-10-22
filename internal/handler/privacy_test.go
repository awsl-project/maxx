package handler

import (
	"bytes"
	"encoding/json"
	"github.com/awsl-project/maxx/internal/domain"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogWriterScrubsCredentialsAtAllSinks(t *testing.T) {
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
	for _, sink := range []string{stdout.String(), string(file), event.Data.(string)} {
		if strings.Contains(sink, "sk-proj-") || !strings.Contains(sink, "[REDACTED]") {
			t.Fatalf("unsafe log: %s", sink)
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
