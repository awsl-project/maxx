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

func TestAdminServiceArchiveInactiveProjectsOnlyDeletesInactiveCandidates(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	projectRepo := sqlite.NewProjectRepository(db)
	requestRepo := sqlite.NewProxyRequestRepository(db)

	neverUsed := &domain.Project{TenantID: 1, Name: "Never Used", Slug: "never-used"}
	oldProject := &domain.Project{TenantID: 1, Name: "Old", Slug: "old"}
	recentProject := &domain.Project{TenantID: 1, Name: "Recent", Slug: "recent"}
	for _, project := range []*domain.Project{neverUsed, oldProject, recentProject} {
		if err := projectRepo.Create(project); err != nil {
			t.Fatalf("create project %s: %v", project.Slug, err)
		}
	}

	oldReq := &domain.ProxyRequest{TenantID: 1, InstanceID: "test", RequestID: "old", SessionID: "old", ClientType: domain.ClientTypeClaude, Status: "COMPLETED", ProjectID: oldProject.ID}
	if err := requestRepo.Create(oldReq); err != nil {
		t.Fatalf("create old request: %v", err)
	}
	oldReq.CreatedAt = time.Now().Add(-45 * 24 * time.Hour)
	if err := requestRepo.Update(oldReq); err != nil {
		t.Fatalf("backdate old request: %v", err)
	}

	recentReq := &domain.ProxyRequest{TenantID: 1, InstanceID: "test", RequestID: "recent", SessionID: "recent", ClientType: domain.ClientTypeClaude, Status: "COMPLETED", ProjectID: recentProject.ID}
	if err := requestRepo.Create(recentReq); err != nil {
		t.Fatalf("create recent request: %v", err)
	}

	svc := NewAdminService(nil, nil, projectRepo, nil, nil, nil, requestRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", nil, nil, nil)
	result, err := svc.ArchiveInactiveProjects(1, 30)
	if err != nil {
		t.Fatalf("ArchiveInactiveProjects: %v", err)
	}
	if result.ArchivedCount != 2 {
		t.Fatalf("ArchivedCount = %d, want 2", result.ArchivedCount)
	}

	if _, err := projectRepo.GetByID(1, neverUsed.ID); err == nil {
		t.Fatalf("never-used project still visible after archive")
	}
	if _, err := projectRepo.GetByID(1, oldProject.ID); err == nil {
		t.Fatalf("old project still visible after archive")
	}
	if _, err := projectRepo.GetByID(1, recentProject.ID); err != nil {
		t.Fatalf("recent project should be preserved: %v", err)
	}
}

func TestAdminServiceArchiveInactiveProjectsFailsClosedWithoutUsageRepository(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	projectRepo := sqlite.NewProjectRepository(db)
	project := &domain.Project{TenantID: 1, Name: "Must Preserve", Slug: "must-preserve"}
	if err := projectRepo.Create(project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	svc := NewAdminService(nil, nil, projectRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", nil, nil, nil)
	if _, err := svc.ArchiveInactiveProjects(1, 30); err == nil {
		t.Fatalf("ArchiveInactiveProjects succeeded without proxyRequestRepo, want fail-closed error")
	}
	if _, err := projectRepo.GetByID(1, project.ID); err != nil {
		t.Fatalf("project should be preserved after fail-closed archive: %v", err)
	}
}
