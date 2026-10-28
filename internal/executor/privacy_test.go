package executor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/privacy"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

type unreadPrivacyBody struct{ reads int }

func (b *unreadPrivacyBody) Read([]byte) (int, error) { b.reads++; return 0, io.EOF }

func TestEarlyRejectionArchivesUseProjectPrivacyWithoutReadingBody(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projects := sqlite.NewProjectRepository(db)
	requests := sqlite.NewProxyRequestRepository(db)
	exec := &Executor{proxyRequestRepo: requests, privacy: privacy.NewService(projects)}
	for _, enabled := range []bool{false, true} {
		project := &domain.Project{TenantID: 1, Name: "early", BlockSecrets: enabled}
		if err := projects.Create(project); err != nil {
			t.Fatal(err)
		}
		for _, fromHeader := range []bool{false, true} {
			body := &unreadPrivacyBody{}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?api_key=query-secret", body)
			req.Header.Set("X-Service-Secret", "header-secret")
			token := &domain.APIToken{ID: 1, TenantID: 1, ProjectID: project.ID}
			if fromHeader {
				token.ProjectID = 0
				req.Header.Set("X-Maxx-Project-ID", strconv.FormatUint(project.ID, 10))
			}
			c := flow.NewCtx(httptest.NewRecorder(), req)
			c.Set(flow.KeyRequestHeaders, req.Header)
			c.Set(flow.KeyRequestURI, req.URL.RequestURI())
			exec.RecordRejectedProxyRequest(c, token, 429, "limit password=Example1234")
			rows, err := requests.List(1, 1, 0)
			if err != nil || len(rows) != 1 {
				t.Fatalf("early rejection missing: %v", err)
			}
			stored, err := requests.GetByID(1, rows[0].ID)
			if err != nil || stored.StatusCode != 429 || stored.ProjectID != project.ID || stored.RequestInfo == nil {
				t.Fatalf("early rejection changed: %+v %v", stored, err)
			}
			if strings.Contains(stored.RequestInfo.URL, "query-secret") == enabled || strings.Contains(stored.Error, "Example1234") == enabled || (stored.RequestInfo.Headers["X-Service-Secret"] == "header-secret") == enabled {
				t.Fatal("early archive did not use project policy")
			}
			if body.reads != 0 || stored.RequestInfo.Body != "" {
				t.Fatal("early rejection read or saved the request body")
			}
		}
	}
}
