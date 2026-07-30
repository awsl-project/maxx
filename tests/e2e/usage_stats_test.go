package e2e_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestGetUsageStats_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/usage-stats?granularity=hour")
	AssertStatus(t, resp, http.StatusOK)

	var stats []any
	DecodeJSON(t, resp, &stats)

	// No usage stats in a fresh environment
	if len(stats) != 0 {
		t.Fatalf("Expected 0 usage stats, got %d", len(stats))
	}
}

func TestGetUsageStats_WithTimeRange(t *testing.T) {
	env := NewTestEnv(t)

	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour).Format(time.RFC3339)
	end := now.Format(time.RFC3339)

	resp := env.AdminGet("/api/admin/usage-stats?granularity=hour&start=" + start + "&end=" + end)
	AssertStatus(t, resp, http.StatusOK)

	var stats []any
	DecodeJSON(t, resp, &stats)

	// Fresh environment should return empty stats even with time range
	if len(stats) != 0 {
		t.Fatalf("Expected 0 usage stats with time range, got %d", len(stats))
	}
}

func TestRecalculateUsageStats(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminPost("/api/admin/usage-stats/recalculate", nil)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["message"] == nil || result["message"] == "" {
		t.Fatal("Expected success message in recalculate response")
	}
}

func TestUsageStatsMonthAndLast24HoursDivergeAfterHistoricalBackfill(t *testing.T) {
	env := NewTestEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)

	request := &sqlite.ProxyRequest{TenantID: 1, ClientType: "openai", ProjectID: 20, Status: "COMPLETED"}
	if err := env.DB.GormDB().Create(request).Error; err != nil {
		t.Fatalf("seed request: %v", err)
	}
	attempts := []*sqlite.ProxyUpstreamAttempt{
		{TenantID: 1, ProxyRequestID: request.ID, ProviderID: 10, Status: "COMPLETED", EndTime: now.Add(-48 * time.Hour).UnixMilli(), ResponseModel: "gpt-5", InputTokenCount: 100},
		{TenantID: 1, ProxyRequestID: request.ID, ProviderID: 10, Status: "COMPLETED", EndTime: now.Add(-25 * time.Hour).UnixMilli(), ResponseModel: "gpt-5", InputTokenCount: 200},
		{TenantID: 1, ProxyRequestID: request.ID, ProviderID: 10, Status: "COMPLETED", EndTime: now.Add(-10 * time.Minute).UnixMilli(), ResponseModel: "gpt-5", InputTokenCount: 300},
	}
	if err := env.DB.GormDB().Create(&attempts).Error; err != nil {
		t.Fatalf("seed attempts: %v", err)
	}

	resp := env.AdminPost("/api/admin/usage-stats/recalculate", nil)
	AssertStatus(t, resp, http.StatusOK)

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	last24Start := now.Add(-24 * time.Hour).Format(time.RFC3339)
	end := now.Format(time.RFC3339)

	monthResp := env.AdminGet("/api/admin/usage-stats?granularity=day&start=" + url.QueryEscape(monthStart) + "&end=" + url.QueryEscape(end))
	AssertStatus(t, monthResp, http.StatusOK)
	last24Resp := env.AdminGet("/api/admin/usage-stats?granularity=hour&start=" + url.QueryEscape(last24Start) + "&end=" + url.QueryEscape(end))
	AssertStatus(t, last24Resp, http.StatusOK)

	var monthStats, last24Stats []struct {
		TotalRequests uint64 `json:"totalRequests"`
	}
	DecodeJSON(t, monthResp, &monthStats)
	DecodeJSON(t, last24Resp, &last24Stats)

	var monthTotal, last24Total uint64
	for _, stat := range monthStats {
		monthTotal += stat.TotalRequests
	}
	for _, stat := range last24Stats {
		last24Total += stat.TotalRequests
	}

	if monthTotal != 3 || last24Total != 1 {
		t.Fatalf("month=%d last24=%d, want 3/1 through admin API after recalculate", monthTotal, last24Total)
	}
}
