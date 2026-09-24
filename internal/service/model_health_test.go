package service

import (
	"context"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type fakeModelHealthRepo struct {
	checks []*domain.ModelHealthCheck
}

func (r *fakeModelHealthRepo) Create(check *domain.ModelHealthCheck) error {
	copy := *check
	r.checks = append(r.checks, &copy)
	return nil
}

func (r *fakeModelHealthRepo) LatestByTargets(tenantID uint64, targets []domain.ModelHealthCheckTarget) (map[string]*domain.ModelHealthCheck, error) {
	result := map[string]*domain.ModelHealthCheck{}
	for _, check := range r.checks {
		if check.TenantID != tenantID {
			continue
		}
		key := check.Model + ":" + string(check.ClientType)
		if prev := result[key]; prev == nil || check.CheckedAt.After(prev.CheckedAt) {
			result[key] = check
		}
	}
	out := map[string]*domain.ModelHealthCheck{}
	for _, target := range targets {
		if check := result[target.Model+":"+string(target.ClientType)]; check != nil {
			out[repository.ModelHealthTargetKey(target.Model, target.ClientType, target.RouteID, target.ProviderID)] = check
		}
	}
	return out, nil
}

func (r *fakeModelHealthRepo) ListByTargetsSince(tenantID uint64, _ []domain.ModelHealthCheckTarget, since time.Time) ([]*domain.ModelHealthCheck, error) {
	var out []*domain.ModelHealthCheck
	for _, check := range r.checks {
		if check.TenantID == tenantID && !check.CheckedAt.Before(since) {
			out = append(out, check)
		}
	}
	return out, nil
}

type fakeModelHealthChecker struct{ calls int }

func (c *fakeModelHealthChecker) ProbeModelHealth(context.Context, uint64, uint64, domain.ModelHealthCheckTarget) domain.ModelHealthProbeResult {
	c.calls++
	return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusOK, LatencyMs: 123}
}

func TestGetUserPanelModelHealthGrid_ProbesStaleTargetAndBuilds24hDots(t *testing.T) {
	repo := &fakeModelHealthRepo{}
	checker := &fakeModelHealthChecker{}
	svc := &AdminService{modelHealthRepo: repo, modelHealthChecker: checker}
	target := domain.ModelHealthCheckTarget{Model: "gpt-5.5", ClientType: domain.ClientTypeOpenAI, RouteID: 1, ProviderID: 2, ProviderName: "newapi"}

	rows, err := svc.GetUserPanelModelHealthGrid(context.Background(), 1, 7, []domain.ModelHealthCheckTarget{target})
	if err != nil {
		t.Fatalf("GetUserPanelModelHealthGrid: %v", err)
	}
	if checker.calls != 1 {
		t.Fatalf("probe calls = %d, want 1", checker.calls)
	}
	if len(rows) != 1 || len(rows[0].Points) != modelHealthBucketCount {
		t.Fatalf("rows = %+v, want one %d-point row", rows, modelHealthBucketCount)
	}
	if rows[0].Current.Status != domain.ModelHealthStatusOK || rows[0].Current.LatencyMs != 123 {
		t.Fatalf("current = %+v, want ok latency", rows[0].Current)
	}
}
