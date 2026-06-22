package service

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestAdminServiceGetProjectsAttachesUsageSummaries(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	projectRepo := sqlite.NewProjectRepository(db)
	requestRepo := sqlite.NewProxyRequestRepository(db)
	project := &domain.Project{TenantID: 1, Name: "Cleanup Candidate", Slug: "cleanup-candidate"}
	if err := projectRepo.Create(project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	req := &domain.ProxyRequest{
		TenantID:     1,
		InstanceID:   "test-instance",
		RequestID:    "req-service-usage",
		SessionID:    "session-service-usage",
		ClientType:   domain.ClientTypeClaude,
		RequestModel: "claude-test",
		Status:       "COMPLETED",
		ProjectID:    project.ID,
	}
	if err := requestRepo.Create(req); err != nil {
		t.Fatalf("create request: %v", err)
	}

	svc := NewAdminService(
		nil,
		nil,
		projectRepo,
		nil,
		nil,
		nil,
		requestRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		nil,
	)

	projects, err := svc.GetProjects(1)
	if err != nil {
		t.Fatalf("get projects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects length = %d, want 1", len(projects))
	}
	got := projects[0]
	if got.LastRequestAt == nil || time.Since(*got.LastRequestAt) > time.Minute {
		t.Fatalf("LastRequestAt = %v, want recent timestamp", got.LastRequestAt)
	}
	if got.LastSuccessfulRequestAt == nil {
		t.Fatalf("LastSuccessfulRequestAt is nil, want completed request timestamp")
	}
	if got.TotalRequestCount != 1 || got.RequestCount30d != 1 || got.SuccessfulRequestCount30d != 1 {
		t.Fatalf("usage counts = total %d recent %d success %d, want 1/1/1", got.TotalRequestCount, got.RequestCount30d, got.SuccessfulRequestCount30d)
	}
}
