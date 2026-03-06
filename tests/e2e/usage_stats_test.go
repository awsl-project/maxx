package e2e_test

import (
	"net/http"
	"testing"
	"time"
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
