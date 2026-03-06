package quota

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type fakeUsageSummaryRepo struct {
	summary    *domain.UsageStatsSummary
	err        error
	called     bool
	lastTenant uint64
	lastFilter repository.UsageStatsFilter
}

func (f *fakeUsageSummaryRepo) GetSummary(tenantID uint64, filter repository.UsageStatsFilter) (*domain.UsageStatsSummary, error) {
	f.called = true
	f.lastTenant = tenantID
	f.lastFilter = filter
	if f.err != nil {
		return nil, f.err
	}
	return f.summary, nil
}

type fakeSettingRepo struct {
	value string
	err   error
}

func (f *fakeSettingRepo) Get(key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.value, nil
}

func TestEvaluateProviderQuota_DisabledSkipsUsageQuery(t *testing.T) {
	provider := &domain.Provider{
		ID: 12,
		Config: &domain.ProviderConfig{
			Quota: &domain.ProviderQuotaConfig{
				Enabled:      false,
				Period:       domain.ProviderQuotaPeriodWeek,
				RequestLimit: 100,
			},
		},
	}
	usageRepo := &fakeUsageSummaryRepo{}
	settingRepo := &fakeSettingRepo{value: "Asia/Singapore"}

	now := time.Date(2026, 3, 6, 16, 0, 0, 0, time.UTC)
	status, err := EvaluateProviderQuota(provider, 1, usageRepo, settingRepo, now)
	if err != nil {
		t.Fatalf("EvaluateProviderQuota() error = %v", err)
	}
	if status == nil {
		t.Fatalf("status is nil")
	}
	if usageRepo.called {
		t.Fatalf("usage repo should not be called when quota is disabled")
	}
	if status.Period != domain.ProviderQuotaPeriodWeek {
		t.Fatalf("period = %s, want %s", status.Period, domain.ProviderQuotaPeriodWeek)
	}
	if !status.HasAnyLimit {
		t.Fatalf("expected HasAnyLimit=true")
	}
}

func TestEvaluateProviderQuota_EnforcedAndExceeded(t *testing.T) {
	provider := &domain.Provider{
		ID: 99,
		Config: &domain.ProviderConfig{
			Quota: &domain.ProviderQuotaConfig{
				Enabled:                 true,
				Period:                  domain.ProviderQuotaPeriodDay,
				RequestLimit:            100,
				TokenLimit:              1000,
				CostLimit:               5000,
				WarningThresholdPercent: 70,
			},
		},
	}
	usageRepo := &fakeUsageSummaryRepo{
		summary: &domain.UsageStatsSummary{
			TotalRequests:     80,
			TotalInputTokens:  500,
			TotalOutputTokens: 700,
			TotalCost:         5100,
		},
	}
	settingRepo := &fakeSettingRepo{value: "UTC"}

	now := time.Date(2026, 3, 6, 10, 30, 0, 0, time.UTC)
	status, err := EvaluateProviderQuota(provider, 7, usageRepo, settingRepo, now)
	if err != nil {
		t.Fatalf("EvaluateProviderQuota() error = %v", err)
	}
	if status == nil {
		t.Fatalf("status is nil")
	}
	if !usageRepo.called {
		t.Fatalf("usage repo should be called when quota is enabled")
	}
	if usageRepo.lastTenant != 7 {
		t.Fatalf("tenantID = %d, want 7", usageRepo.lastTenant)
	}
	if usageRepo.lastFilter.ProviderID == nil || *usageRepo.lastFilter.ProviderID != 99 {
		t.Fatalf("filter.ProviderID mismatch")
	}
	if status.Requests == nil || status.Tokens == nil || status.Cost == nil {
		t.Fatalf("expected all metric statuses to be present")
	}
	if !status.Requests.Warning || status.Requests.Exceeded {
		t.Fatalf("requests metric expected warning without exceeded")
	}
	if !status.Tokens.Exceeded {
		t.Fatalf("tokens metric should be exceeded")
	}
	if !status.Cost.Exceeded {
		t.Fatalf("cost metric should be exceeded")
	}
	if !status.Warning || !status.Exceeded {
		t.Fatalf("status warning/exceeded not set correctly")
	}
}

func TestEvaluateProviderQuota_WeekWindowByTimezone(t *testing.T) {
	provider := &domain.Provider{
		ID: 1,
		Config: &domain.ProviderConfig{
			Quota: &domain.ProviderQuotaConfig{
				Enabled:      true,
				Period:       domain.ProviderQuotaPeriodWeek,
				RequestLimit: 10,
			},
		},
	}
	usageRepo := &fakeUsageSummaryRepo{summary: &domain.UsageStatsSummary{}}
	settingRepo := &fakeSettingRepo{value: "Asia/Singapore"}

	// Friday, 2026-03-06 12:00 UTC => 20:00 Singapore
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	status, err := EvaluateProviderQuota(provider, 1, usageRepo, settingRepo, now)
	if err != nil {
		t.Fatalf("EvaluateProviderQuota() error = %v", err)
	}
	if status == nil {
		t.Fatalf("status is nil")
	}
	if status.Timezone != "Asia/Singapore" {
		t.Fatalf("timezone = %s, want Asia/Singapore", status.Timezone)
	}

	// Week starts Monday 00:00 in Singapore => 2026-03-01 16:00:00 UTC
	wantStart := time.Date(2026, 3, 1, 16, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 3, 8, 16, 0, 0, 0, time.UTC)
	if !status.PeriodStart.Equal(wantStart) {
		t.Fatalf("periodStart = %s, want %s", status.PeriodStart, wantStart)
	}
	if !status.PeriodEnd.Equal(wantEnd) {
		t.Fatalf("periodEnd = %s, want %s", status.PeriodEnd, wantEnd)
	}
	if usageRepo.lastFilter.StartTime == nil || !usageRepo.lastFilter.StartTime.Equal(wantStart) {
		t.Fatalf("filter.StartTime mismatch")
	}
	if usageRepo.lastFilter.EndTime == nil || !usageRepo.lastFilter.EndTime.Equal(wantEnd) {
		t.Fatalf("filter.EndTime mismatch")
	}
}
