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

func TestEarlyRejectionArchivesUseGlobalPrivacyWithoutReadingBody(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projects := sqlite.NewProjectRepository(db)
	requests := sqlite.NewProxyRequestRepository(db)
	exec := &Executor{proxyRequestRepo: requests, privacy: privacy.NewService(sqlite.NewSystemSettingRepository(db))}
	for _, enabled := range []bool{false, true} {
		cfg := privacy.DefaultConfig()
		cfg.Enabled = enabled
		if _, err := exec.privacy.SetConfiguration(cfg); err != nil {
			t.Fatal(err)
		}
		project := &domain.Project{TenantID: 1, Name: "early"}
		if err := projects.Create(project); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name                                 string
			tokenID, headerID, projectID, wantID uint64
		}{
			{name: "unassigned"},
			{name: "token binding", tokenID: project.ID, wantID: project.ID},
			{name: "header alone stays unresolved", headerID: project.ID},
			{name: "header does not override token", headerID: project.ID + 100, tokenID: project.ID, wantID: project.ID},
			{name: "resolved project precedes token", projectID: project.ID, tokenID: project.ID + 100, wantID: project.ID},
		} {
			t.Run(strconv.FormatBool(enabled)+"/"+tc.name, func(t *testing.T) {
				body := &unreadPrivacyBody{}
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?api_key=query-secret", body)
				req.Header.Set("X-Service-Secret", "header-secret")
				token := &domain.APIToken{ID: 1, TenantID: 1, ProjectID: tc.tokenID}
				if tc.headerID != 0 {
					req.Header.Set("X-Maxx-Project-ID", strconv.FormatUint(tc.headerID, 10))
				}
				c := flow.NewCtx(httptest.NewRecorder(), req)
				c.Set(flow.KeyProjectID, tc.projectID)
				c.Set(flow.KeyRequestHeaders, req.Header)
				c.Set(flow.KeyRequestURI, req.URL.RequestURI())
				exec.RecordRejectedProxyRequest(c, token, 429, "limit password=Example1234")
				rows, err := requests.List(1, 1, 0)
				if err != nil || len(rows) != 1 {
					t.Fatalf("early rejection missing: %v", err)
				}
				stored, err := requests.GetByID(1, rows[0].ID)
				if err != nil || stored.StatusCode != 429 || stored.ProjectID != tc.wantID || stored.RequestInfo == nil {
					t.Fatalf("early rejection changed: %+v %v", stored, err)
				}
				if strings.Contains(stored.RequestInfo.URL, "query-secret") == enabled || strings.Contains(stored.Error, "Example1234") == enabled || (stored.RequestInfo.Headers["X-Service-Secret"] == "header-secret") == enabled {
					t.Fatal("early archive did not use global policy")
				}
				if body.reads != 0 || stored.RequestInfo.Body != "" {
					t.Fatal("early rejection read or saved the request body")
				}
			})
		}
	}
}
