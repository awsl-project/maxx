package service

import (
	"context"
	"sync"
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

type fakeModelHealthChecker struct {
	mu    sync.Mutex
	calls int
}

func (c *fakeModelHealthChecker) ProbeModelHealth(context.Context, uint64, uint64, domain.ModelHealthCheckTarget) domain.ModelHealthProbeResult {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusOK, LatencyMs: 123}
}

func (c *fakeModelHealthChecker) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type slowModelHealthChecker struct {
	delay time.Duration
	mu    sync.Mutex
	calls int
}

func (c *slowModelHealthChecker) ProbeModelHealth(ctx context.Context, _ uint64, _ uint64, _ domain.ModelHealthCheckTarget) domain.ModelHealthProbeResult {
	select {
	case <-time.After(c.delay):
	case <-ctx.Done():
		return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusError, Error: ctx.Err().Error()}
	}
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return domain.ModelHealthProbeResult{Status: domain.ModelHealthStatusOK, LatencyMs: int64(c.delay / time.Millisecond)}
}

func (c *slowModelHealthChecker) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
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
	if calls := checker.callCount(); calls != 1 {
		t.Fatalf("probe calls = %d, want 1", calls)
	}
	if len(rows) != 1 || len(rows[0].Points) != modelHealthBucketCount {
		t.Fatalf("rows = %+v, want one %d-point row", rows, modelHealthBucketCount)
	}
	if rows[0].Current.Status != domain.ModelHealthStatusOK || rows[0].Current.LatencyMs != 123 {
		t.Fatalf("current = %+v, want ok latency", rows[0].Current)
	}
}

func TestGetUserPanelModelHealthGrid_ReturnsUnknownRowsWhenProbeCapReached(t *testing.T) {
	repo := &fakeModelHealthRepo{}
	checker := &fakeModelHealthChecker{}
	svc := &AdminService{modelHealthRepo: repo, modelHealthChecker: checker}
	targets := make([]domain.ModelHealthCheckTarget, 0, modelHealthMaxProbesPerRequest+3)
	for i := 0; i < modelHealthMaxProbesPerRequest+3; i++ {
		targets = append(targets, domain.ModelHealthCheckTarget{Model: string(rune('a' + i)), ClientType: domain.ClientTypeOpenAI, RouteID: uint64(i + 1), ProviderID: uint64(i + 101), ProviderName: "newapi"})
	}

	rows, err := svc.GetUserPanelModelHealthGrid(context.Background(), 1, 7, targets)
	if err != nil {
		t.Fatalf("GetUserPanelModelHealthGrid: %v", err)
	}
	if calls := checker.callCount(); calls != modelHealthMaxProbesPerRequest {
		t.Fatalf("probe calls = %d, want capped %d", calls, modelHealthMaxProbesPerRequest)
	}
	if len(rows) != len(targets) {
		t.Fatalf("rows = %d, want all %d targets represented", len(rows), len(targets))
	}
	unknown := 0
	for _, row := range rows {
		if row.Current.Status == domain.ModelHealthStatusUnknown {
			unknown++
		}
	}
	if unknown == 0 {
		t.Fatalf("want capped, unprobed targets to stay visible as unknown rows")
	}
}

func TestGetUserPanelModelHealthGrid_ProbesStaleTargetsConcurrently(t *testing.T) {
	repo := &fakeModelHealthRepo{}
	checker := &slowModelHealthChecker{delay: 50 * time.Millisecond}
	svc := &AdminService{modelHealthRepo: repo, modelHealthChecker: checker}
	targets := make([]domain.ModelHealthCheckTarget, 0, modelHealthProbeConcurrency)
	for i := 0; i < modelHealthProbeConcurrency; i++ {
		targets = append(targets, domain.ModelHealthCheckTarget{Model: string(rune('a' + i)), ClientType: domain.ClientTypeOpenAI, RouteID: uint64(i + 1), ProviderID: uint64(i + 101), ProviderName: "no-switch-provider"})
	}

	started := time.Now()
	rows, err := svc.GetUserPanelModelHealthGrid(context.Background(), 1, 7, targets)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("GetUserPanelModelHealthGrid: %v", err)
	}
	if len(rows) != len(targets) {
		t.Fatalf("rows = %d, want %d", len(rows), len(targets))
	}
	if calls := checker.callCount(); calls != len(targets) {
		t.Fatalf("probe calls = %d, want %d", calls, len(targets))
	}
	serialFloor := checker.delay * time.Duration(len(targets))
	if elapsed >= serialFloor {
		t.Fatalf("health probes took %s, want less than serial floor %s", elapsed, serialFloor)
	}
}
