package sqlite

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func seedProjectUsageRequest(t *testing.T, db *DB, repo *ProxyRequestRepository, tenantID, projectID uint64, status string, createdAt time.Time) {
	t.Helper()
	req := &domain.ProxyRequest{
		TenantID:     tenantID,
		InstanceID:   "test-instance",
		RequestID:    "req-usage",
		SessionID:    "session-usage",
		ClientType:   domain.ClientTypeClaude,
		RequestModel: "claude-test",
		Status:       status,
		ProjectID:    projectID,
	}
	if err := repo.Create(req); err != nil {
		t.Fatalf("create request: %v", err)
	}
	if err := db.gorm.Model(&ProxyRequest{}).Where("id = ?", req.ID).Updates(map[string]any{
		"created_at": toTimestamp(createdAt),
		"updated_at": toTimestamp(createdAt),
	}).Error; err != nil {
		t.Fatalf("backdate request: %v", err)
	}
}

func TestProxyRequestGetProjectUsageSummaries(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 6, 22, 8, 0, 0, 0, time.UTC)
	projectRepo := NewProjectRepository(db)
	requestRepo := NewProxyRequestRepository(db)

	projectA := &domain.Project{TenantID: 1, Name: "Active", Slug: "active"}
	projectB := &domain.Project{TenantID: 1, Name: "Failed Only", Slug: "failed-only"}
	otherTenantProject := &domain.Project{TenantID: 2, Name: "Other", Slug: "other"}
	for _, project := range []*domain.Project{projectA, projectB, otherTenantProject} {
		if err := projectRepo.Create(project); err != nil {
			t.Fatalf("create project %s: %v", project.Name, err)
		}
	}

	seedProjectUsageRequest(t, db, requestRepo, 1, projectA.ID, "COMPLETED", now.Add(-10*24*time.Hour))
	seedProjectUsageRequest(t, db, requestRepo, 1, projectA.ID, "FAILED", now.Add(-5*24*time.Hour))
	seedProjectUsageRequest(t, db, requestRepo, 1, projectB.ID, "FAILED", now.Add(-40*24*time.Hour))
	seedProjectUsageRequest(t, db, requestRepo, 2, otherTenantProject.ID, "COMPLETED", now.Add(-1*24*time.Hour))

	summaries, err := requestRepo.GetProjectUsageSummaries(1, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("get summaries: %v", err)
	}

	active := summaries[projectA.ID]
	if active.TotalRequestCount != 2 || active.RequestCount30d != 2 || active.SuccessfulRequestCount30d != 1 {
		t.Fatalf("active counts = %+v, want total 2 recent 2 recent success 1", active)
	}
	if active.LastRequestAt == nil || !active.LastRequestAt.Equal(now.Add(-5*24*time.Hour)) {
		t.Fatalf("active last request = %v", active.LastRequestAt)
	}
	if active.LastSuccessfulRequestAt == nil || !active.LastSuccessfulRequestAt.Equal(now.Add(-10*24*time.Hour)) {
		t.Fatalf("active last success = %v", active.LastSuccessfulRequestAt)
	}

	failedOnly := summaries[projectB.ID]
	if failedOnly.TotalRequestCount != 1 || failedOnly.RequestCount30d != 0 || failedOnly.SuccessfulRequestCount30d != 0 {
		t.Fatalf("failed-only counts = %+v, want total 1 recent 0 recent success 0", failedOnly)
	}
	if failedOnly.LastSuccessfulRequestAt != nil {
		t.Fatalf("failed-only last success = %v, want nil", failedOnly.LastSuccessfulRequestAt)
	}
	if _, ok := summaries[otherTenantProject.ID]; ok {
		t.Fatalf("summary leaked other tenant project id %d", otherTenantProject.ID)
	}

	filtered, err := requestRepo.GetProjectUsageSummaries(1, now.Add(-30*24*time.Hour), projectB.ID)
	if err != nil {
		t.Fatalf("get filtered summaries: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered summary length = %d, want 1", len(filtered))
	}
	if _, ok := filtered[projectB.ID]; !ok {
		t.Fatalf("filtered summary missing project id %d", projectB.ID)
	}
	if _, ok := filtered[projectA.ID]; ok {
		t.Fatalf("filtered summary included unrequested project id %d", projectA.ID)
	}
}
