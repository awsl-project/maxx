package sqlite

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

func sumUsageStatsRequests(stats []*domain.UsageStats) uint64 {
	var total uint64
	for _, s := range stats {
		total += s.TotalRequests
	}
	return total
}

func seedUsageStatsRegressionAttempts(t *testing.T, db *DB, now time.Time) {
	t.Helper()

	request := &ProxyRequest{TenantID: 1, ClientType: "openai", ProjectID: 20, Status: "COMPLETED"}
	if err := db.gorm.Create(request).Error; err != nil {
		t.Fatalf("seed request: %v", err)
	}

	attempts := []*ProxyUpstreamAttempt{
		{TenantID: 1, ProxyRequestID: request.ID, ProviderID: 10, Status: "COMPLETED", EndTime: toTimestamp(now.Add(-48 * time.Hour)), InputTokenCount: 100},
		{TenantID: 1, ProxyRequestID: request.ID, ProviderID: 10, Status: "COMPLETED", EndTime: toTimestamp(now.Add(-25 * time.Hour)), InputTokenCount: 200},
		{TenantID: 1, ProxyRequestID: request.ID, ProviderID: 10, Status: "COMPLETED", EndTime: toTimestamp(now.Add(-10 * time.Minute)), InputTokenCount: 300},
	}
	if err := db.gorm.Create(&attempts).Error; err != nil {
		t.Fatalf("seed attempts: %v", err)
	}
}

func TestAggregateAndRollUpInitialRunBackfillsHistoricalAttempts(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://" + t.TempDir() + "/stats.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := NewUsageStatsRepository(db)
	now := time.Now().UTC().Truncate(time.Minute)
	seedUsageStatsRegressionAttempts(t, db, now)

	for range repo.AggregateAndRollUp(0) {
	}

	historicalStart := now.Add(-72 * time.Hour)
	last24Start := now.Add(-24 * time.Hour)
	monthStats, err := repo.Query(1, repository.UsageStatsFilter{
		Granularity: domain.GranularityDay,
		StartTime:   &historicalStart,
		EndTime:     &now,
	})
	if err != nil {
		t.Fatalf("month query: %v", err)
	}
	last24Stats, err := repo.Query(1, repository.UsageStatsFilter{
		Granularity: domain.GranularityHour,
		StartTime:   &last24Start,
		EndTime:     &now,
	})
	if err != nil {
		t.Fatalf("last24 query: %v", err)
	}

	monthTotal := sumUsageStatsRequests(monthStats)
	last24Total := sumUsageStatsRequests(last24Stats)
	if monthTotal <= last24Total {
		t.Fatalf("month=%d last24=%d, want month to include older historical attempts after first aggregation", monthTotal, last24Total)
	}
	if monthTotal != 3 || last24Total != 1 {
		t.Fatalf("month=%d last24=%d, want 3/1 without realtime overlap double counting", monthTotal, last24Total)
	}
}

func TestQueryRealtimeBackfillDoesNotDoubleCountCompletedHour(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://" + t.TempDir() + "/stats.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := NewUsageStatsRepository(db)
	now := time.Now().UTC().Truncate(time.Minute)
	seedUsageStatsRegressionAttempts(t, db, now)

	progress := make(chan domain.Progress, 16)
	if err := repo.ClearAndRecalculateWithProgress(0, progress); err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	close(progress)

	historicalStart := now.Add(-72 * time.Hour)
	last24Start := now.Add(-24 * time.Hour)
	monthStats, err := repo.Query(1, repository.UsageStatsFilter{
		Granularity: domain.GranularityDay,
		StartTime:   &historicalStart,
		EndTime:     &now,
	})
	if err != nil {
		t.Fatalf("month query: %v", err)
	}
	last24Stats, err := repo.Query(1, repository.UsageStatsFilter{
		Granularity: domain.GranularityHour,
		StartTime:   &last24Start,
		EndTime:     &now,
	})
	if err != nil {
		t.Fatalf("last24 query: %v", err)
	}

	monthTotal := sumUsageStatsRequests(monthStats)
	last24Total := sumUsageStatsRequests(last24Stats)
	if monthTotal != 3 || last24Total != 1 {
		t.Fatalf("month=%d last24=%d, want 3/1 after recalc without raw overlap double counting", monthTotal, last24Total)
	}
}
