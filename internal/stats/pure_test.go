package stats

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestTruncateToGranularity(t *testing.T) {
	// 2024-01-17 14:35:42 UTC (Wednesday)
	testTime := time.Date(2024, 1, 17, 14, 35, 42, 123456789, time.UTC)

	tests := []struct {
		name        string
		granularity domain.Granularity
		expected    time.Time
	}{
		{
			name:        "minute",
			granularity: domain.GranularityMinute,
			expected:    time.Date(2024, 1, 17, 14, 35, 0, 0, time.UTC),
		},
		{
			name:        "hour",
			granularity: domain.GranularityHour,
			expected:    time.Date(2024, 1, 17, 14, 0, 0, 0, time.UTC),
		},
		{
			name:        "day",
			granularity: domain.GranularityDay,
			expected:    time.Date(2024, 1, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "month",
			granularity: domain.GranularityMonth,
			expected:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "unknown granularity defaults to hour",
			granularity: domain.Granularity("unknown"),
			expected:    time.Date(2024, 1, 17, 14, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToGranularity(testTime, tt.granularity, time.UTC)
			if !result.Equal(tt.expected) {
				t.Errorf("TruncateToGranularity(%v, %v, UTC) = %v, want %v",
					testTime, tt.granularity, result, tt.expected)
			}
		})
	}
}

func TestTruncateToGranularity_Timezone(t *testing.T) {
	shanghai, _ := time.LoadLocation("Asia/Shanghai")
	tokyo, _ := time.LoadLocation("Asia/Tokyo")

	// 2024-01-17 02:30:00 UTC = 2024-01-17 10:30:00 Shanghai = 2024-01-17 11:30:00 Tokyo
	testTimeUTC := time.Date(2024, 1, 17, 2, 30, 0, 0, time.UTC)

	tests := []struct {
		name        string
		loc         *time.Location
		granularity domain.Granularity
		expected    time.Time
	}{
		{
			name:        "day in Shanghai timezone",
			loc:         shanghai,
			granularity: domain.GranularityDay,
			expected:    time.Date(2024, 1, 17, 0, 0, 0, 0, shanghai),
		},
		{
			name:        "hour in Shanghai timezone",
			loc:         shanghai,
			granularity: domain.GranularityHour,
			expected:    time.Date(2024, 1, 17, 10, 0, 0, 0, shanghai),
		},
		{
			name:        "minute in Shanghai timezone",
			loc:         shanghai,
			granularity: domain.GranularityMinute,
			expected:    time.Date(2024, 1, 17, 10, 30, 0, 0, shanghai),
		},
		{
			name:        "day in Tokyo timezone",
			loc:         tokyo,
			granularity: domain.GranularityDay,
			expected:    time.Date(2024, 1, 17, 0, 0, 0, 0, tokyo),
		},
		{
			name:        "month in Shanghai timezone",
			loc:         shanghai,
			granularity: domain.GranularityMonth,
			expected:    time.Date(2024, 1, 1, 0, 0, 0, 0, shanghai),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToGranularity(testTimeUTC, tt.granularity, tt.loc)
			if !result.Equal(tt.expected) {
				t.Errorf("TruncateToGranularity(%v, %v, %v) = %v, want %v",
					testTimeUTC, tt.granularity, tt.loc, result, tt.expected)
			}
		})
	}
}

func TestTruncateToGranularity_DayBoundary(t *testing.T) {
	shanghai, _ := time.LoadLocation("Asia/Shanghai")

	// 2024-01-17 23:30:00 UTC = 2024-01-18 07:30:00 Shanghai
	// This is a different day in Shanghai than in UTC
	testTimeUTC := time.Date(2024, 1, 17, 23, 30, 0, 0, time.UTC)

	utcDay := TruncateToGranularity(testTimeUTC, domain.GranularityDay, time.UTC)
	shanghaiDay := TruncateToGranularity(testTimeUTC, domain.GranularityDay, shanghai)

	expectedUTCDay := time.Date(2024, 1, 17, 0, 0, 0, 0, time.UTC)
	expectedShanghaiDay := time.Date(2024, 1, 18, 0, 0, 0, 0, shanghai)

	if !utcDay.Equal(expectedUTCDay) {
		t.Errorf("UTC day = %v, want %v", utcDay, expectedUTCDay)
	}
	if !shanghaiDay.Equal(expectedShanghaiDay) {
		t.Errorf("Shanghai day = %v, want %v", shanghaiDay, expectedShanghaiDay)
	}
}

func TestAggregateAttempts_Empty(t *testing.T) {
	result := AggregateAttempts(nil, time.UTC)
	if result != nil {
		t.Errorf("expected nil for empty records, got %v", result)
	}

	result = AggregateAttempts([]AttemptRecord{}, time.UTC)
	if result != nil {
		t.Errorf("expected nil for empty slice, got %v", result)
	}
}

func TestAggregateAttempts_Single(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 15, 0, time.UTC)

	records := []AttemptRecord{
		{
			EndTime:      baseTime,
			ProviderID:   1,
			ProjectID:    2,
			RouteID:      3,
			APITokenID:   4,
			ClientType:   "claude",
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  100,
			OutputTokens: 50,
			DurationMs:   1000,
			CacheRead:    10,
			CacheWrite:   5,
			Cost:         1000,
		},
	}

	result := AggregateAttempts(records, time.UTC)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s := result[0]
	if s.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", s.TotalRequests)
	}
	if s.SuccessfulRequests != 1 {
		t.Errorf("SuccessfulRequests = %d, want 1", s.SuccessfulRequests)
	}
	if s.FailedRequests != 0 {
		t.Errorf("FailedRequests = %d, want 0", s.FailedRequests)
	}
	if s.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", s.InputTokens)
	}
	if s.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", s.OutputTokens)
	}
	if s.TotalDurationMs != 1000 {
		t.Errorf("TotalDurationMs = %d, want 1000", s.TotalDurationMs)
	}
	if s.CacheRead != 10 {
		t.Errorf("CacheRead = %d, want 10", s.CacheRead)
	}
	if s.CacheWrite != 5 {
		t.Errorf("CacheWrite = %d, want 5", s.CacheWrite)
	}
	if s.Cost != 1000 {
		t.Errorf("Cost = %d, want 1000", s.Cost)
	}
	if s.ProviderID != 1 {
		t.Errorf("ProviderID = %d, want 1", s.ProviderID)
	}
	if s.ProjectID != 2 {
		t.Errorf("ProjectID = %d, want 2", s.ProjectID)
	}
	if s.RouteID != 3 {
		t.Errorf("RouteID = %d, want 3", s.RouteID)
	}
	if s.APITokenID != 4 {
		t.Errorf("APITokenID = %d, want 4", s.APITokenID)
	}
	if s.ClientType != "claude" {
		t.Errorf("ClientType = %s, want claude", s.ClientType)
	}
	if s.Model != "claude-3" {
		t.Errorf("Model = %s, want claude-3", s.Model)
	}
	if s.Granularity != domain.GranularityMinute {
		t.Errorf("Granularity = %v, want minute", s.Granularity)
	}
}

func TestAggregateAttempts_SameMinute(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	records := []AttemptRecord{
		{
			EndTime:      baseTime.Add(10 * time.Second),
			ProviderID:   1,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  100,
			OutputTokens: 50,
			Cost:         1000,
		},
		{
			EndTime:      baseTime.Add(20 * time.Second),
			ProviderID:   1,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  200,
			OutputTokens: 100,
			Cost:         2000,
		},
		{
			EndTime:    baseTime.Add(30 * time.Second),
			ProviderID: 1,
			Model:      "claude-3",
			IsFailed:   true,
			Cost:       0,
		},
	}

	result := AggregateAttempts(records, time.UTC)

	if len(result) != 1 {
		t.Fatalf("expected 1 aggregated result, got %d", len(result))
	}

	s := result[0]
	if s.TotalRequests != 3 {
		t.Errorf("TotalRequests = %d, want 3", s.TotalRequests)
	}
	if s.SuccessfulRequests != 2 {
		t.Errorf("SuccessfulRequests = %d, want 2", s.SuccessfulRequests)
	}
	if s.FailedRequests != 1 {
		t.Errorf("FailedRequests = %d, want 1", s.FailedRequests)
	}
	if s.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", s.InputTokens)
	}
	if s.OutputTokens != 150 {
		t.Errorf("OutputTokens = %d, want 150", s.OutputTokens)
	}
	if s.Cost != 3000 {
		t.Errorf("Cost = %d, want 3000", s.Cost)
	}
}

func TestAggregateAttempts_DifferentMinutes(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	records := []AttemptRecord{
		{
			EndTime:      baseTime,
			ProviderID:   1,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  100,
		},
		{
			EndTime:      baseTime.Add(1 * time.Minute),
			ProviderID:   1,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  200,
		},
	}

	result := AggregateAttempts(records, time.UTC)

	if len(result) != 2 {
		t.Fatalf("expected 2 results for different minutes, got %d", len(result))
	}
}

func TestAggregateAttempts_DifferentProviders(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	records := []AttemptRecord{
		{
			EndTime:      baseTime,
			ProviderID:   1,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  100,
		},
		{
			EndTime:      baseTime,
			ProviderID:   2,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  200,
		},
	}

	result := AggregateAttempts(records, time.UTC)

	if len(result) != 2 {
		t.Fatalf("expected 2 results for different providers, got %d", len(result))
	}
}

func TestAggregateAttempts_DifferentModels(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	records := []AttemptRecord{
		{
			EndTime:      baseTime,
			ProviderID:   1,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  100,
		},
		{
			EndTime:      baseTime,
			ProviderID:   1,
			Model:        "gpt-4",
			IsSuccessful: true,
			InputTokens:  200,
		},
	}

	result := AggregateAttempts(records, time.UTC)

	if len(result) != 2 {
		t.Fatalf("expected 2 results for different models, got %d", len(result))
	}
}

func TestAggregateAttempts_DifferentDimensions(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	// Test all dimension variations
	records := []AttemptRecord{
		{EndTime: baseTime, ProviderID: 1, ProjectID: 1, RouteID: 1, APITokenID: 1, ClientType: "a", Model: "m", InputTokens: 1},
		{EndTime: baseTime, ProviderID: 1, ProjectID: 2, RouteID: 1, APITokenID: 1, ClientType: "a", Model: "m", InputTokens: 2}, // diff project
		{EndTime: baseTime, ProviderID: 1, ProjectID: 1, RouteID: 2, APITokenID: 1, ClientType: "a", Model: "m", InputTokens: 3}, // diff route
		{EndTime: baseTime, ProviderID: 1, ProjectID: 1, RouteID: 1, APITokenID: 2, ClientType: "a", Model: "m", InputTokens: 4}, // diff token
		{EndTime: baseTime, ProviderID: 1, ProjectID: 1, RouteID: 1, APITokenID: 1, ClientType: "b", Model: "m", InputTokens: 5}, // diff client
	}

	result := AggregateAttempts(records, time.UTC)

	if len(result) != 5 {
		t.Fatalf("expected 5 results for different dimensions, got %d", len(result))
	}

	var total uint64
	for _, s := range result {
		total += s.InputTokens
	}
	if total != 15 {
		t.Errorf("total input tokens = %d, want 15", total)
	}
}

func TestAggregateAttempts_WithTimezone(t *testing.T) {
	shanghai, _ := time.LoadLocation("Asia/Shanghai")

	// 2024-01-17 23:30:00 UTC = 2024-01-18 07:30:00 Shanghai
	// These should be in different minute buckets when using Shanghai timezone
	utcTime := time.Date(2024, 1, 17, 23, 30, 30, 0, time.UTC)

	records := []AttemptRecord{
		{
			EndTime:      utcTime,
			ProviderID:   1,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  100,
		},
	}

	result := AggregateAttempts(records, shanghai)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// The time bucket should be 2024-01-18 07:30:00 Shanghai
	expected := time.Date(2024, 1, 18, 7, 30, 0, 0, shanghai)
	if !result[0].TimeBucket.Equal(expected) {
		t.Errorf("TimeBucket = %v, want %v", result[0].TimeBucket, expected)
	}
}

func TestRollUp_Empty(t *testing.T) {
	result := RollUp(nil, domain.GranularityHour, time.UTC)
	if result != nil {
		t.Errorf("expected nil for empty stats, got %v", result)
	}

	result = RollUp([]*domain.UsageStats{}, domain.GranularityHour, time.UTC)
	if result != nil {
		t.Errorf("expected nil for empty slice, got %v", result)
	}
}

func TestRollUp_MinuteToHour(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	minuteStats := []*domain.UsageStats{
		{
			Granularity:        domain.GranularityMinute,
			TimeBucket:         baseTime,
			ProviderID:         1,
			Model:              "claude-3",
			TotalRequests:      10,
			SuccessfulRequests: 8,
			FailedRequests:     2,
			TotalDurationMs:    10000,
			InputTokens:        1000,
			OutputTokens:       500,
			CacheRead:          100,
			CacheWrite:         50,
			Cost:               10000,
		},
		{
			Granularity:   domain.GranularityMinute,
			TimeBucket:    baseTime.Add(15 * time.Minute),
			ProviderID:    1,
			Model:         "claude-3",
			TotalRequests: 5,
			InputTokens:   500,
			OutputTokens:  250,
			Cost:          5000,
		},
		{
			Granularity:   domain.GranularityMinute,
			TimeBucket:    baseTime.Add(30 * time.Minute),
			ProviderID:    1,
			Model:         "claude-3",
			TotalRequests: 8,
			InputTokens:   800,
			OutputTokens:  400,
			Cost:          8000,
		},
	}

	result := RollUp(minuteStats, domain.GranularityHour, time.UTC)

	if len(result) != 1 {
		t.Fatalf("expected 1 hour bucket, got %d", len(result))
	}

	h := result[0]
	if h.TotalRequests != 23 {
		t.Errorf("TotalRequests = %d, want 23", h.TotalRequests)
	}
	if h.InputTokens != 2300 {
		t.Errorf("InputTokens = %d, want 2300", h.InputTokens)
	}
	if h.OutputTokens != 1150 {
		t.Errorf("OutputTokens = %d, want 1150", h.OutputTokens)
	}
	if h.Cost != 23000 {
		t.Errorf("Cost = %d, want 23000", h.Cost)
	}
	if h.Granularity != domain.GranularityHour {
		t.Errorf("Granularity = %v, want hour", h.Granularity)
	}
}

func TestRollUp_MinuteToDay(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	minuteStats := []*domain.UsageStats{
		{Granularity: domain.GranularityMinute, TimeBucket: baseTime, ProviderID: 1, TotalRequests: 10, InputTokens: 1000},
		{Granularity: domain.GranularityMinute, TimeBucket: baseTime.Add(60 * time.Minute), ProviderID: 1, TotalRequests: 5, InputTokens: 500},
		{Granularity: domain.GranularityMinute, TimeBucket: baseTime.Add(120 * time.Minute), ProviderID: 1, TotalRequests: 8, InputTokens: 800},
	}

	result := RollUp(minuteStats, domain.GranularityDay, time.UTC)

	if len(result) != 1 {
		t.Fatalf("expected 1 day bucket, got %d", len(result))
	}

	if result[0].TotalRequests != 23 {
		t.Errorf("TotalRequests = %d, want 23", result[0].TotalRequests)
	}
	if result[0].InputTokens != 2300 {
		t.Errorf("InputTokens = %d, want 2300", result[0].InputTokens)
	}
}

func TestRollUp_DayToMonth(t *testing.T) {
	day1 := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	day15 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	day25 := time.Date(2024, 1, 25, 0, 0, 0, 0, time.UTC)

	dayStats := []*domain.UsageStats{
		{Granularity: domain.GranularityDay, TimeBucket: day1, ProviderID: 1, TotalRequests: 100, InputTokens: 10000},
		{Granularity: domain.GranularityDay, TimeBucket: day15, ProviderID: 1, TotalRequests: 200, InputTokens: 20000},
		{Granularity: domain.GranularityDay, TimeBucket: day25, ProviderID: 1, TotalRequests: 300, InputTokens: 30000},
	}

	result := RollUp(dayStats, domain.GranularityMonth, time.UTC)

	if len(result) != 1 {
		t.Fatalf("expected 1 month bucket, got %d", len(result))
	}

	if result[0].TotalRequests != 600 {
		t.Errorf("TotalRequests = %d, want 600", result[0].TotalRequests)
	}
	if result[0].InputTokens != 60000 {
		t.Errorf("InputTokens = %d, want 60000", result[0].InputTokens)
	}
}

func TestRollUp_PreservesAggregationKey(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	stats := []*domain.UsageStats{
		{
			Granularity: domain.GranularityMinute,
			TimeBucket:  baseTime,
			ProviderID:  1,
			ProjectID:   1,
			RouteID:     1,
			APITokenID:  1,
			ClientType:  "claude",
			Model:       "claude-3",
			InputTokens: 100,
		},
		{
			Granularity: domain.GranularityMinute,
			TimeBucket:  baseTime.Add(5 * time.Minute),
			ProviderID:  1,
			ProjectID:   1,
			RouteID:     1,
			APITokenID:  1,
			ClientType:  "claude",
			Model:       "claude-3",
			InputTokens: 100,
		},
		{
			Granularity: domain.GranularityMinute,
			TimeBucket:  baseTime,
			ProviderID:  2, // Different provider
			ProjectID:   1,
			RouteID:     1,
			APITokenID:  1,
			ClientType:  "openai",
			Model:       "gpt-4",
			InputTokens: 200,
		},
	}

	result := RollUp(stats, domain.GranularityHour, time.UTC)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	var p1, p2 *domain.UsageStats
	for _, s := range result {
		switch s.ProviderID {
		case 1:
			p1 = s
		case 2:
			p2 = s
		}
	}

	if p1 == nil || p2 == nil {
		t.Fatal("missing expected provider stats")
	}

	if p1.InputTokens != 200 {
		t.Errorf("provider 1 input tokens = %d, want 200", p1.InputTokens)
	}
	if p2.InputTokens != 200 {
		t.Errorf("provider 2 input tokens = %d, want 200", p2.InputTokens)
	}
}

func TestAggregateAttempts_DifferentTenants(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	records := []AttemptRecord{
		{
			EndTime:      baseTime,
			TenantID:     1,
			ProviderID:   1,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  100,
		},
		{
			EndTime:      baseTime,
			TenantID:     2,
			ProviderID:   1,
			Model:        "claude-3",
			IsSuccessful: true,
			InputTokens:  200,
		},
	}

	result := AggregateAttempts(records, time.UTC)
	if len(result) != 2 {
		t.Fatalf("expected 2 aggregated results for different tenants, got %d", len(result))
	}

	tenantTotals := map[uint64]uint64{}
	for _, s := range result {
		tenantTotals[s.TenantID] += s.InputTokens
	}
	if tenantTotals[1] != 100 {
		t.Errorf("tenant 1 input tokens = %d, want 100", tenantTotals[1])
	}
	if tenantTotals[2] != 200 {
		t.Errorf("tenant 2 input tokens = %d, want 200", tenantTotals[2])
	}
}

func TestRollUp_DifferentTenants(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	minuteStats := []*domain.UsageStats{
		{
			TenantID:      1,
			Granularity:   domain.GranularityMinute,
			TimeBucket:    baseTime,
			ProviderID:    1,
			Model:         "claude-3",
			TotalRequests: 1,
			InputTokens:   100,
		},
		{
			TenantID:      2,
			Granularity:   domain.GranularityMinute,
			TimeBucket:    baseTime.Add(5 * time.Minute),
			ProviderID:    1,
			Model:         "claude-3",
			TotalRequests: 1,
			InputTokens:   200,
		},
	}

	result := RollUp(minuteStats, domain.GranularityHour, time.UTC)
	if len(result) != 2 {
		t.Fatalf("expected 2 hourly stats for different tenants, got %d", len(result))
	}

	tenantTotals := map[uint64]uint64{}
	for _, s := range result {
		tenantTotals[s.TenantID] += s.InputTokens
	}
	if tenantTotals[1] != 100 {
		t.Errorf("tenant 1 input tokens = %d, want 100", tenantTotals[1])
	}
	if tenantTotals[2] != 200 {
		t.Errorf("tenant 2 input tokens = %d, want 200", tenantTotals[2])
	}
}

func TestMergeStats_DifferentTenants(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	list1 := []*domain.UsageStats{
		{
			TenantID:      1,
			Granularity:   domain.GranularityHour,
			TimeBucket:    baseTime,
			ProviderID:    1,
			Model:         "claude-3",
			TotalRequests: 1,
			InputTokens:   100,
		},
	}
	list2 := []*domain.UsageStats{
		{
			TenantID:      2,
			Granularity:   domain.GranularityHour,
			TimeBucket:    baseTime,
			ProviderID:    1,
			Model:         "claude-3",
			TotalRequests: 1,
			InputTokens:   200,
		},
	}

	result := MergeStats(list1, list2)
	if len(result) != 2 {
		t.Fatalf("expected 2 merged results for different tenants, got %d", len(result))
	}

	tenantTotals := map[uint64]uint64{}
	for _, s := range result {
		tenantTotals[s.TenantID] += s.InputTokens
	}
	if tenantTotals[1] != 100 {
		t.Errorf("tenant 1 input tokens = %d, want 100", tenantTotals[1])
	}
	if tenantTotals[2] != 200 {
		t.Errorf("tenant 2 input tokens = %d, want 200", tenantTotals[2])
	}
}

func TestRollUp_WithTimezone(t *testing.T) {
	shanghai, _ := time.LoadLocation("Asia/Shanghai")

	// 2024-01-17 23:00:00 UTC = 2024-01-18 07:00:00 Shanghai
	// 2024-01-18 01:00:00 UTC = 2024-01-18 09:00:00 Shanghai
	// Both should be in the same day in Shanghai, but different days in UTC
	time1 := time.Date(2024, 1, 17, 23, 0, 0, 0, time.UTC)
	time2 := time.Date(2024, 1, 18, 1, 0, 0, 0, time.UTC)

	hourStats := []*domain.UsageStats{
		{Granularity: domain.GranularityHour, TimeBucket: time1, ProviderID: 1, TotalRequests: 100, InputTokens: 10000},
		{Granularity: domain.GranularityHour, TimeBucket: time2, ProviderID: 1, TotalRequests: 50, InputTokens: 5000},
	}

	// With UTC - should be 2 different days
	resultUTC := RollUp(hourStats, domain.GranularityDay, time.UTC)
	if len(resultUTC) != 2 {
		t.Errorf("expected 2 day buckets in UTC, got %d", len(resultUTC))
	}

	// With Shanghai - should be 1 day
	resultShanghai := RollUp(hourStats, domain.GranularityDay, shanghai)
	if len(resultShanghai) != 1 {
		t.Errorf("expected 1 day bucket in Shanghai, got %d", len(resultShanghai))
	}
	if resultShanghai[0].TotalRequests != 150 {
		t.Errorf("Shanghai total requests = %d, want 150", resultShanghai[0].TotalRequests)
	}
}

func TestMergeStats_Empty(t *testing.T) {
	result := MergeStats()
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}

	result = MergeStats(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result for nil slices, got %d", len(result))
	}
}

func TestMergeStats_SingleList(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	list := []*domain.UsageStats{
		{Granularity: domain.GranularityHour, TimeBucket: baseTime, ProviderID: 1, InputTokens: 100},
		{Granularity: domain.GranularityHour, TimeBucket: baseTime, ProviderID: 2, InputTokens: 200},
	}

	result := MergeStats(list)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
}

func TestMergeStats_MergeMatchingKeys(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	list1 := []*domain.UsageStats{
		{
			Granularity:        domain.GranularityHour,
			TimeBucket:         baseTime,
			ProviderID:         1,
			Model:              "claude-3",
			TotalRequests:      10,
			SuccessfulRequests: 8,
			FailedRequests:     2,
			TotalDurationMs:    10000,
			InputTokens:        100,
			OutputTokens:       50,
			CacheRead:          10,
			CacheWrite:         5,
			Cost:               1000,
		},
	}

	list2 := []*domain.UsageStats{
		{
			Granularity:        domain.GranularityHour,
			TimeBucket:         baseTime,
			ProviderID:         1,
			Model:              "claude-3",
			TotalRequests:      5,
			SuccessfulRequests: 5,
			FailedRequests:     0,
			TotalDurationMs:    5000,
			InputTokens:        200,
			OutputTokens:       100,
			CacheRead:          20,
			CacheWrite:         10,
			Cost:               2000,
		},
	}

	result := MergeStats(list1, list2)

	if len(result) != 1 {
		t.Fatalf("expected 1 merged result, got %d", len(result))
	}

	s := result[0]
	if s.TotalRequests != 15 {
		t.Errorf("TotalRequests = %d, want 15", s.TotalRequests)
	}
	if s.SuccessfulRequests != 13 {
		t.Errorf("SuccessfulRequests = %d, want 13", s.SuccessfulRequests)
	}
	if s.FailedRequests != 2 {
		t.Errorf("FailedRequests = %d, want 2", s.FailedRequests)
	}
	if s.TotalDurationMs != 15000 {
		t.Errorf("TotalDurationMs = %d, want 15000", s.TotalDurationMs)
	}
	if s.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", s.InputTokens)
	}
	if s.OutputTokens != 150 {
		t.Errorf("OutputTokens = %d, want 150", s.OutputTokens)
	}
	if s.CacheRead != 30 {
		t.Errorf("CacheRead = %d, want 30", s.CacheRead)
	}
	if s.CacheWrite != 15 {
		t.Errorf("CacheWrite = %d, want 15", s.CacheWrite)
	}
	if s.Cost != 3000 {
		t.Errorf("Cost = %d, want 3000", s.Cost)
	}
}

func TestMergeStats_DifferentKeys(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	list1 := []*domain.UsageStats{
		{Granularity: domain.GranularityHour, TimeBucket: baseTime, ProviderID: 1, InputTokens: 100},
	}

	list2 := []*domain.UsageStats{
		{Granularity: domain.GranularityHour, TimeBucket: baseTime, ProviderID: 2, InputTokens: 200},
	}

	list3 := []*domain.UsageStats{
		{Granularity: domain.GranularityDay, TimeBucket: baseTime, ProviderID: 1, InputTokens: 300}, // Different granularity
	}

	result := MergeStats(list1, list2, list3)

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	var total uint64
	for _, s := range result {
		total += s.InputTokens
	}
	if total != 600 {
		t.Errorf("total input tokens = %d, want 600", total)
	}
}

func TestMergeStats_DoesNotModifyOriginal(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	original := &domain.UsageStats{
		Granularity: domain.GranularityHour,
		TimeBucket:  baseTime,
		ProviderID:  1,
		InputTokens: 100,
	}

	list1 := []*domain.UsageStats{original}
	list2 := []*domain.UsageStats{
		{Granularity: domain.GranularityHour, TimeBucket: baseTime, ProviderID: 1, InputTokens: 200},
	}

	_ = MergeStats(list1, list2)

	// Original should not be modified
	if original.InputTokens != 100 {
		t.Errorf("original was modified: InputTokens = %d, want 100", original.InputTokens)
	}
}

func TestSumStats_Empty(t *testing.T) {
	totalReq, successReq, failedReq, inputTokens, outputTokens, cacheRead, cacheWrite, cost := SumStats(nil)

	if totalReq != 0 || successReq != 0 || failedReq != 0 || inputTokens != 0 ||
		outputTokens != 0 || cacheRead != 0 || cacheWrite != 0 || cost != 0 {
		t.Errorf("expected all zeros for empty stats")
	}
}

func TestSumStats(t *testing.T) {
	stats := []*domain.UsageStats{
		{
			TotalRequests:      10,
			SuccessfulRequests: 8,
			FailedRequests:     2,
			InputTokens:        1000,
			OutputTokens:       500,
			CacheRead:          100,
			CacheWrite:         50,
			Cost:               10000,
		},
		{
			TotalRequests:      5,
			SuccessfulRequests: 5,
			FailedRequests:     0,
			InputTokens:        500,
			OutputTokens:       250,
			CacheRead:          50,
			CacheWrite:         25,
			Cost:               5000,
		},
	}

	totalReq, successReq, failedReq, inputTokens, outputTokens, cacheRead, cacheWrite, cost := SumStats(stats)

	if totalReq != 15 {
		t.Errorf("totalReq = %d, want 15", totalReq)
	}
	if successReq != 13 {
		t.Errorf("successReq = %d, want 13", successReq)
	}
	if failedReq != 2 {
		t.Errorf("failedReq = %d, want 2", failedReq)
	}
	if inputTokens != 1500 {
		t.Errorf("inputTokens = %d, want 1500", inputTokens)
	}
	if outputTokens != 750 {
		t.Errorf("outputTokens = %d, want 750", outputTokens)
	}
	if cacheRead != 150 {
		t.Errorf("cacheRead = %d, want 150", cacheRead)
	}
	if cacheWrite != 75 {
		t.Errorf("cacheWrite = %d, want 75", cacheWrite)
	}
	if cost != 15000 {
		t.Errorf("cost = %d, want 15000", cost)
	}
}

func TestSummarize_Empty(t *testing.T) {
	got := Summarize(nil)
	if got == nil {
		t.Fatal("Summarize(nil) returned nil, want zero-value summary")
	}
	if got.TotalRequests != 0 || got.SuccessRate != 0 {
		t.Errorf("empty summary should be all zeros, got %+v", got)
	}
}

func TestSummarize(t *testing.T) {
	in := []*domain.UsageStats{
		{
			TotalRequests:      10,
			SuccessfulRequests: 8,
			FailedRequests:     2,
			InputTokens:        1000,
			OutputTokens:       500,
			CacheRead:          100,
			CacheWrite:         50,
			Cost:               10000,
		},
		{
			TotalRequests:      5,
			SuccessfulRequests: 5,
			InputTokens:        500,
			OutputTokens:       250,
			CacheRead:          50,
			CacheWrite:         25,
			Cost:               5000,
		},
	}
	got := Summarize(in)
	if got.TotalRequests != 15 || got.SuccessfulRequests != 13 || got.FailedRequests != 2 {
		t.Errorf("requests: %+v", got)
	}
	if got.TotalInputTokens != 1500 || got.TotalOutputTokens != 750 {
		t.Errorf("tokens: %+v", got)
	}
	if got.TotalCacheRead != 150 || got.TotalCacheWrite != 75 || got.TotalCost != 15000 {
		t.Errorf("cache/cost: %+v", got)
	}
	// 13/15 = 86.666...% — 这里既验证 SuccessRate 计算位置,也防止以后改成 float32 之类的精度回退。
	wantRate := 13.0 / 15.0 * 100
	if got.SuccessRate != wantRate {
		t.Errorf("SuccessRate = %v, want %v", got.SuccessRate, wantRate)
	}
}

func TestGroupByProvider_Empty(t *testing.T) {
	result := GroupByProvider(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestGroupByProvider_SkipsZeroProvider(t *testing.T) {
	stats := []*domain.UsageStats{
		{ProviderID: 0, TotalRequests: 100, InputTokens: 10000},
		{ProviderID: 1, TotalRequests: 50, InputTokens: 5000},
	}

	result := GroupByProvider(stats)

	if len(result) != 1 {
		t.Fatalf("expected 1 provider (skipping 0), got %d", len(result))
	}
	if result[0] != nil {
		t.Error("provider 0 should not be in result")
	}
	if result[1] == nil {
		t.Fatal("provider 1 should be in result")
	}
	if result[1].TotalRequests != 50 {
		t.Errorf("provider 1 TotalRequests = %d, want 50", result[1].TotalRequests)
	}
}

func TestGroupByProvider(t *testing.T) {
	stats := []*domain.UsageStats{
		{
			ProviderID:         1,
			TotalRequests:      10,
			SuccessfulRequests: 8,
			FailedRequests:     2,
			InputTokens:        1000,
			OutputTokens:       500,
			CacheRead:          100,
			CacheWrite:         50,
			Cost:               10000,
		},
		{
			ProviderID:         1,
			TotalRequests:      5,
			SuccessfulRequests: 5,
			InputTokens:        500,
			OutputTokens:       250,
			CacheRead:          50,
			CacheWrite:         25,
			Cost:               5000,
		},
		{
			ProviderID:         2,
			TotalRequests:      3,
			SuccessfulRequests: 3,
			InputTokens:        300,
			OutputTokens:       150,
			CacheRead:          30,
			CacheWrite:         15,
			Cost:               3000,
		},
	}

	result := GroupByProvider(stats)

	if len(result) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(result))
	}

	p1 := result[1]
	if p1 == nil {
		t.Fatal("provider 1 not found")
	}
	if p1.ProviderID != 1 {
		t.Errorf("ProviderID = %d, want 1", p1.ProviderID)
	}
	if p1.TotalRequests != 15 {
		t.Errorf("provider 1 TotalRequests = %d, want 15", p1.TotalRequests)
	}
	if p1.SuccessfulRequests != 13 {
		t.Errorf("provider 1 SuccessfulRequests = %d, want 13", p1.SuccessfulRequests)
	}
	if p1.FailedRequests != 2 {
		t.Errorf("provider 1 FailedRequests = %d, want 2", p1.FailedRequests)
	}
	if p1.TotalInputTokens != 1500 {
		t.Errorf("provider 1 TotalInputTokens = %d, want 1500", p1.TotalInputTokens)
	}
	if p1.TotalOutputTokens != 750 {
		t.Errorf("provider 1 TotalOutputTokens = %d, want 750", p1.TotalOutputTokens)
	}
	if p1.TotalCacheRead != 150 {
		t.Errorf("provider 1 TotalCacheRead = %d, want 150", p1.TotalCacheRead)
	}
	if p1.TotalCacheWrite != 75 {
		t.Errorf("provider 1 TotalCacheWrite = %d, want 75", p1.TotalCacheWrite)
	}
	if p1.TotalCost != 15000 {
		t.Errorf("provider 1 TotalCost = %d, want 15000", p1.TotalCost)
	}

	// Success rate: 13/15 * 100 = 86.67%
	expectedRate := float64(13) / float64(15) * 100
	if p1.SuccessRate != expectedRate {
		t.Errorf("provider 1 SuccessRate = %f, want %f", p1.SuccessRate, expectedRate)
	}

	p2 := result[2]
	if p2 == nil {
		t.Fatal("provider 2 not found")
	}
	if p2.TotalRequests != 3 {
		t.Errorf("provider 2 TotalRequests = %d, want 3", p2.TotalRequests)
	}
	if p2.SuccessRate != 100 {
		t.Errorf("provider 2 SuccessRate = %f, want 100", p2.SuccessRate)
	}
}

func TestGroupByProvider_ZeroRequests(t *testing.T) {
	stats := []*domain.UsageStats{
		{ProviderID: 1, TotalRequests: 0, SuccessfulRequests: 0},
	}

	result := GroupByProvider(stats)

	if result[1].SuccessRate != 0 {
		t.Errorf("SuccessRate = %f, want 0 for zero requests", result[1].SuccessRate)
	}
}

func TestFilterByGranularity_Empty(t *testing.T) {
	result := FilterByGranularity(nil, domain.GranularityHour)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestFilterByGranularity(t *testing.T) {
	stats := []*domain.UsageStats{
		{Granularity: domain.GranularityMinute, InputTokens: 100},
		{Granularity: domain.GranularityHour, InputTokens: 200},
		{Granularity: domain.GranularityMinute, InputTokens: 300},
		{Granularity: domain.GranularityDay, InputTokens: 400},
	}

	result := FilterByGranularity(stats, domain.GranularityMinute)

	if len(result) != 2 {
		t.Fatalf("expected 2 minute stats, got %d", len(result))
	}

	var total uint64
	for _, s := range result {
		if s.Granularity != domain.GranularityMinute {
			t.Errorf("unexpected granularity: %v", s.Granularity)
		}
		total += s.InputTokens
	}
	if total != 400 {
		t.Errorf("total input = %d, want 400", total)
	}
}

func TestFilterByGranularity_NoMatch(t *testing.T) {
	stats := []*domain.UsageStats{
		{Granularity: domain.GranularityMinute, InputTokens: 100},
		{Granularity: domain.GranularityHour, InputTokens: 200},
	}

	result := FilterByGranularity(stats, domain.GranularityMonth)

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestFilterByTimeRange_Empty(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)
	result := FilterByTimeRange(nil, baseTime, baseTime.Add(time.Hour))
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestFilterByTimeRange(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	stats := []*domain.UsageStats{
		{TimeBucket: baseTime, InputTokens: 100},
		{TimeBucket: baseTime.Add(1 * time.Hour), InputTokens: 200},
		{TimeBucket: baseTime.Add(2 * time.Hour), InputTokens: 300},
		{TimeBucket: baseTime.Add(3 * time.Hour), InputTokens: 400},
	}

	// Filter [10:00, 12:00) - should include 10:00 and 11:00
	result := FilterByTimeRange(stats, baseTime, baseTime.Add(2*time.Hour))

	if len(result) != 2 {
		t.Fatalf("expected 2 stats, got %d", len(result))
	}

	var total uint64
	for _, s := range result {
		total += s.InputTokens
	}
	if total != 300 {
		t.Errorf("total input = %d, want 300", total)
	}
}

func TestFilterByTimeRange_InclusiveStart(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	stats := []*domain.UsageStats{
		{TimeBucket: baseTime, InputTokens: 100},
	}

	result := FilterByTimeRange(stats, baseTime, baseTime.Add(time.Hour))

	if len(result) != 1 {
		t.Errorf("expected 1 stat (start is inclusive), got %d", len(result))
	}
}

func TestFilterByTimeRange_ExclusiveEnd(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	stats := []*domain.UsageStats{
		{TimeBucket: baseTime.Add(time.Hour), InputTokens: 100},
	}

	result := FilterByTimeRange(stats, baseTime, baseTime.Add(time.Hour))

	if len(result) != 0 {
		t.Errorf("expected 0 stats (end is exclusive), got %d", len(result))
	}
}

func TestFilterByTimeRange_NoMatch(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	stats := []*domain.UsageStats{
		{TimeBucket: baseTime.Add(-1 * time.Hour), InputTokens: 100},
		{TimeBucket: baseTime.Add(3 * time.Hour), InputTokens: 200},
	}

	result := FilterByTimeRange(stats, baseTime, baseTime.Add(2*time.Hour))

	if len(result) != 0 {
		t.Errorf("expected 0 stats, got %d", len(result))
	}
}

// Integration test: verify full aggregation pipeline
func TestAggregationPipeline_TokensCorrectlyAggregated(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	// Simulate 100 requests, each with 100 input tokens and 50 output tokens
	// spread across 10 minutes in the same hour
	var records []AttemptRecord
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			records = append(records, AttemptRecord{
				EndTime:      baseTime.Add(time.Duration(i)*time.Minute + time.Duration(j)*time.Second),
				ProviderID:   1,
				Model:        "claude-3",
				IsSuccessful: true,
				InputTokens:  100,
				OutputTokens: 50,
				Cost:         1000,
			})
		}
	}

	// Aggregate to minute
	minuteStats := AggregateAttempts(records, time.UTC)

	// Verify minute aggregation
	var totalMinuteTokens uint64
	for _, s := range minuteStats {
		totalMinuteTokens += s.InputTokens
	}
	expectedTokens := uint64(100 * 100) // 100 requests * 100 tokens
	if totalMinuteTokens != expectedTokens {
		t.Errorf("minute input tokens = %d, want %d", totalMinuteTokens, expectedTokens)
	}

	// Roll up to hour
	hourStats := RollUp(minuteStats, domain.GranularityHour, time.UTC)

	if len(hourStats) != 1 {
		t.Fatalf("expected 1 hour bucket, got %d", len(hourStats))
	}

	h := hourStats[0]
	if h.InputTokens != expectedTokens {
		t.Errorf("hour input tokens = %d, want %d", h.InputTokens, expectedTokens)
	}
	if h.TotalRequests != 100 {
		t.Errorf("hour total requests = %d, want 100", h.TotalRequests)
	}

	// Roll up to day
	dayStats := RollUp(hourStats, domain.GranularityDay, time.UTC)

	if len(dayStats) != 1 {
		t.Fatalf("expected 1 day bucket, got %d", len(dayStats))
	}

	d := dayStats[0]
	if d.InputTokens != expectedTokens {
		t.Errorf("day input tokens = %d, want %d (no data loss)", d.InputTokens, expectedTokens)
	}

	// Roll up to month
	monthStats := RollUp(dayStats, domain.GranularityMonth, time.UTC)

	if len(monthStats) != 1 {
		t.Fatalf("expected 1 month bucket, got %d", len(monthStats))
	}

	m := monthStats[0]
	if m.InputTokens != expectedTokens {
		t.Errorf("month input tokens = %d, want %d (no data loss)", m.InputTokens, expectedTokens)
	}
}

// TestFullAggregationPipeline tests the complete aggregation pipeline
// that AggregateAndRollUp performs: minute → hour → day → month
func TestFullAggregationPipeline(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	// Create test records spanning multiple minutes
	records := []AttemptRecord{
		{EndTime: baseTime, ProviderID: 1, Model: "claude-3", IsSuccessful: true, InputTokens: 100, OutputTokens: 50, Cost: 1000, DurationMs: 500},
		{EndTime: baseTime.Add(30 * time.Second), ProviderID: 1, Model: "claude-3", IsSuccessful: true, InputTokens: 200, OutputTokens: 100, Cost: 2000, DurationMs: 600},
		{EndTime: baseTime.Add(1 * time.Minute), ProviderID: 1, Model: "claude-3", IsFailed: true, InputTokens: 50, OutputTokens: 0, Cost: 0, DurationMs: 100},
		{EndTime: baseTime.Add(2 * time.Minute), ProviderID: 2, Model: "gpt-4", IsSuccessful: true, InputTokens: 300, OutputTokens: 150, Cost: 5000, DurationMs: 800},
	}

	// Step 1: Aggregate to minute
	minuteStats := AggregateAttempts(records, time.UTC)

	// Verify: should have 3 minute buckets (10:30, 10:31, 10:32)
	// But provider/model combinations mean more entries
	if len(minuteStats) < 3 {
		t.Errorf("expected at least 3 minute stats, got %d", len(minuteStats))
	}

	// Verify totals
	totalReq, successReq, failedReq, inputTokens, outputTokens, _, _, cost := SumStats(minuteStats)
	if totalReq != 4 {
		t.Errorf("total requests = %d, want 4", totalReq)
	}
	if successReq != 3 {
		t.Errorf("successful requests = %d, want 3", successReq)
	}
	if failedReq != 1 {
		t.Errorf("failed requests = %d, want 1", failedReq)
	}
	if inputTokens != 650 {
		t.Errorf("input tokens = %d, want 650", inputTokens)
	}
	if outputTokens != 300 {
		t.Errorf("output tokens = %d, want 300", outputTokens)
	}
	if cost != 8000 {
		t.Errorf("cost = %d, want 8000", cost)
	}

	// Step 2: Roll up to hour
	hourStats := RollUp(minuteStats, domain.GranularityHour, time.UTC)

	// Verify totals preserved
	totalReq2, _, _, inputTokens2, _, _, _, cost2 := SumStats(hourStats)
	if totalReq2 != totalReq {
		t.Errorf("hour total requests = %d, want %d (data loss)", totalReq2, totalReq)
	}
	if inputTokens2 != inputTokens {
		t.Errorf("hour input tokens = %d, want %d (data loss)", inputTokens2, inputTokens)
	}
	if cost2 != cost {
		t.Errorf("hour cost = %d, want %d (data loss)", cost2, cost)
	}

	// Step 3: Roll up to day
	dayStats := RollUp(hourStats, domain.GranularityDay, time.UTC)

	totalReq3, _, _, inputTokens3, _, _, _, cost3 := SumStats(dayStats)
	if totalReq3 != totalReq {
		t.Errorf("day total requests = %d, want %d (data loss)", totalReq3, totalReq)
	}
	if inputTokens3 != inputTokens {
		t.Errorf("day input tokens = %d, want %d (data loss)", inputTokens3, inputTokens)
	}
	if cost3 != cost {
		t.Errorf("day cost = %d, want %d (data loss)", cost3, cost)
	}

	// Step 4: Roll up to month
	monthStats := RollUp(dayStats, domain.GranularityMonth, time.UTC)

	totalReq4, _, _, inputTokens4, _, _, _, cost4 := SumStats(monthStats)
	if totalReq4 != totalReq {
		t.Errorf("month total requests = %d, want %d (data loss)", totalReq4, totalReq)
	}
	if inputTokens4 != inputTokens {
		t.Errorf("month input tokens = %d, want %d (data loss)", inputTokens4, inputTokens)
	}
	if cost4 != cost {
		t.Errorf("month cost = %d, want %d (data loss)", cost4, cost)
	}
}

// TestFullAggregationPipeline_PreservesProviderDimension tests that
// provider dimension is preserved through the entire aggregation pipeline
func TestFullAggregationPipeline_PreservesProviderDimension(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	// Create records for 2 different providers
	records := []AttemptRecord{
		{EndTime: baseTime, ProviderID: 1, Model: "claude-3", IsSuccessful: true, InputTokens: 100, Cost: 1000},
		{EndTime: baseTime, ProviderID: 1, Model: "claude-3", IsSuccessful: true, InputTokens: 100, Cost: 1000},
		{EndTime: baseTime, ProviderID: 2, Model: "gpt-4", IsSuccessful: true, InputTokens: 200, Cost: 3000},
	}

	// Aggregate through the entire pipeline
	minuteStats := AggregateAttempts(records, time.UTC)
	hourStats := RollUp(minuteStats, domain.GranularityHour, time.UTC)
	dayStats := RollUp(hourStats, domain.GranularityDay, time.UTC)
	monthStats := RollUp(dayStats, domain.GranularityMonth, time.UTC)

	// Group by provider and verify
	providerStats := GroupByProvider(monthStats)

	if len(providerStats) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providerStats))
	}

	p1 := providerStats[1]
	if p1 == nil {
		t.Fatal("provider 1 not found")
	}
	if p1.TotalRequests != 2 {
		t.Errorf("provider 1 requests = %d, want 2", p1.TotalRequests)
	}
	if p1.TotalInputTokens != 200 {
		t.Errorf("provider 1 input tokens = %d, want 200", p1.TotalInputTokens)
	}
	if p1.TotalCost != 2000 {
		t.Errorf("provider 1 cost = %d, want 2000", p1.TotalCost)
	}

	p2 := providerStats[2]
	if p2 == nil {
		t.Fatal("provider 2 not found")
	}
	if p2.TotalRequests != 1 {
		t.Errorf("provider 2 requests = %d, want 1", p2.TotalRequests)
	}
	if p2.TotalInputTokens != 200 {
		t.Errorf("provider 2 input tokens = %d, want 200", p2.TotalInputTokens)
	}
	if p2.TotalCost != 3000 {
		t.Errorf("provider 2 cost = %d, want 3000", p2.TotalCost)
	}
}

// TestFullAggregationPipeline_WithTimezone tests aggregation with timezone
func TestFullAggregationPipeline_WithTimezone(t *testing.T) {
	shanghai, _ := time.LoadLocation("Asia/Shanghai")

	// 2024-01-17 23:30 UTC = 2024-01-18 07:30 Shanghai
	// 2024-01-18 00:30 UTC = 2024-01-18 08:30 Shanghai
	// In UTC these are different days, in Shanghai they're the same day
	records := []AttemptRecord{
		{EndTime: time.Date(2024, 1, 17, 23, 30, 0, 0, time.UTC), ProviderID: 1, IsSuccessful: true, InputTokens: 100},
		{EndTime: time.Date(2024, 1, 18, 0, 30, 0, 0, time.UTC), ProviderID: 1, IsSuccessful: true, InputTokens: 200},
	}

	// Aggregate with Shanghai timezone
	minuteStats := AggregateAttempts(records, shanghai)
	hourStats := RollUp(minuteStats, domain.GranularityHour, shanghai)
	dayStats := RollUp(hourStats, domain.GranularityDay, shanghai)

	// In Shanghai timezone, both records should be on 2024-01-18
	if len(dayStats) != 1 {
		t.Errorf("expected 1 day bucket in Shanghai timezone, got %d", len(dayStats))
	}

	totalReq, _, _, inputTokens, _, _, _, _ := SumStats(dayStats)
	if totalReq != 2 {
		t.Errorf("total requests = %d, want 2", totalReq)
	}
	if inputTokens != 300 {
		t.Errorf("input tokens = %d, want 300", inputTokens)
	}

	// Now aggregate with UTC - should be 2 different days
	minuteStatsUTC := AggregateAttempts(records, time.UTC)
	hourStatsUTC := RollUp(minuteStatsUTC, domain.GranularityHour, time.UTC)
	dayStatsUTC := RollUp(hourStatsUTC, domain.GranularityDay, time.UTC)

	if len(dayStatsUTC) != 2 {
		t.Errorf("expected 2 day buckets in UTC, got %d", len(dayStatsUTC))
	}
}

// TestFullAggregationPipeline_AllFieldsPreserved tests that all numeric fields
// are correctly summed through the pipeline
func TestFullAggregationPipeline_AllFieldsPreserved(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	records := []AttemptRecord{
		{
			EndTime:      baseTime,
			ProviderID:   1,
			IsSuccessful: true,
			DurationMs:   1000,
			InputTokens:  100,
			OutputTokens: 50,
			CacheRead:    10,
			CacheWrite:   5,
			Cost:         1000,
		},
		{
			EndTime:      baseTime.Add(time.Minute),
			ProviderID:   1,
			IsSuccessful: true,
			DurationMs:   2000,
			InputTokens:  200,
			OutputTokens: 100,
			CacheRead:    20,
			CacheWrite:   10,
			Cost:         2000,
		},
		{
			EndTime:    baseTime.Add(2 * time.Minute),
			ProviderID: 1,
			IsFailed:   true,
			DurationMs: 500,
		},
	}

	// Full pipeline
	minuteStats := AggregateAttempts(records, time.UTC)
	hourStats := RollUp(minuteStats, domain.GranularityHour, time.UTC)
	dayStats := RollUp(hourStats, domain.GranularityDay, time.UTC)
	monthStats := RollUp(dayStats, domain.GranularityMonth, time.UTC)

	// Check all fields are preserved at month level
	totalReq, successReq, failedReq, inputTokens, outputTokens, cacheRead, cacheWrite, cost := SumStats(monthStats)

	if totalReq != 3 {
		t.Errorf("totalReq = %d, want 3", totalReq)
	}
	if successReq != 2 {
		t.Errorf("successReq = %d, want 2", successReq)
	}
	if failedReq != 1 {
		t.Errorf("failedReq = %d, want 1", failedReq)
	}
	if inputTokens != 300 {
		t.Errorf("inputTokens = %d, want 300", inputTokens)
	}
	if outputTokens != 150 {
		t.Errorf("outputTokens = %d, want 150", outputTokens)
	}
	if cacheRead != 30 {
		t.Errorf("cacheRead = %d, want 30", cacheRead)
	}
	if cacheWrite != 15 {
		t.Errorf("cacheWrite = %d, want 15", cacheWrite)
	}
	if cost != 3000 {
		t.Errorf("cost = %d, want 3000", cost)
	}
}

// TestFullAggregationPipeline_MultipleModels tests aggregation with multiple models
func TestFullAggregationPipeline_MultipleModels(t *testing.T) {
	baseTime := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)

	records := []AttemptRecord{
		{EndTime: baseTime, ProviderID: 1, Model: "claude-3-opus", IsSuccessful: true, InputTokens: 100, Cost: 5000},
		{EndTime: baseTime, ProviderID: 1, Model: "claude-3-sonnet", IsSuccessful: true, InputTokens: 100, Cost: 1000},
		{EndTime: baseTime, ProviderID: 1, Model: "claude-3-opus", IsSuccessful: true, InputTokens: 100, Cost: 5000},
	}

	minuteStats := AggregateAttempts(records, time.UTC)
	monthStats := RollUp(
		RollUp(
			RollUp(minuteStats, domain.GranularityHour, time.UTC),
			domain.GranularityDay, time.UTC),
		domain.GranularityMonth, time.UTC)

	// Should have 2 entries: one for each model
	if len(monthStats) != 2 {
		t.Errorf("expected 2 model entries, got %d", len(monthStats))
	}

	// Find opus and sonnet stats
	var opusStats, sonnetStats *domain.UsageStats
	for _, s := range monthStats {
		switch s.Model {
		case "claude-3-opus":
			opusStats = s
		case "claude-3-sonnet":
			sonnetStats = s
		}
	}

	if opusStats == nil {
		t.Fatal("opus stats not found")
	}
	if opusStats.TotalRequests != 2 {
		t.Errorf("opus requests = %d, want 2", opusStats.TotalRequests)
	}
	if opusStats.Cost != 10000 {
		t.Errorf("opus cost = %d, want 10000", opusStats.Cost)
	}

	if sonnetStats == nil {
		t.Fatal("sonnet stats not found")
	}
	if sonnetStats.TotalRequests != 1 {
		t.Errorf("sonnet requests = %d, want 1", sonnetStats.TotalRequests)
	}
	if sonnetStats.Cost != 1000 {
		t.Errorf("sonnet cost = %d, want 1000", sonnetStats.Cost)
	}
}

func TestTopModelsByRequests_Empty(t *testing.T) {
	if got := TopModelsByRequests(nil, 3); len(got) != 0 {
		t.Errorf("nil input → %v, want empty", got)
	}
	if got := TopModelsByRequests([]*domain.UsageStats{}, 3); len(got) != 0 {
		t.Errorf("empty input → %v, want empty", got)
	}
}

func TestTopModelsByRequests_ZeroLimit(t *testing.T) {
	in := []*domain.UsageStats{{Model: "a", TotalRequests: 10}}
	if got := TopModelsByRequests(in, 0); len(got) != 0 {
		t.Errorf("limit=0 → %v, want empty", got)
	}
	if got := TopModelsByRequests(in, -1); len(got) != 0 {
		t.Errorf("limit=-1 → %v, want empty", got)
	}
}

func TestTopModelsByRequests_IgnoresEmptyModel(t *testing.T) {
	in := []*domain.UsageStats{
		{Model: "", TotalRequests: 100, InputTokens: 1000},
		{Model: "real", TotalRequests: 10, InputTokens: 100},
	}
	got := TopModelsByRequests(in, 5)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (empty model ignored)", len(got))
	}
	if got[0].Model != "real" {
		t.Errorf("model = %q, want real", got[0].Model)
	}
}

func TestTopModelsByRequests_AggregatesAcrossBuckets(t *testing.T) {
	in := []*domain.UsageStats{
		{Model: "claude", TotalRequests: 10, InputTokens: 100, OutputTokens: 50, CacheRead: 5, CacheWrite: 2},
		{Model: "claude", TotalRequests: 5, InputTokens: 50, OutputTokens: 25},
		{Model: "gpt", TotalRequests: 20, InputTokens: 200},
	}
	got := TopModelsByRequests(in, 5)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// gpt is top: 20 > 15
	if got[0].Model != "gpt" || got[0].Requests != 20 || got[0].Tokens != 200 {
		t.Errorf("top = %+v, want gpt/20/200", got[0])
	}
	// claude aggregated: 15 req, tokens = 100+50+50+25+5+2 = 232
	if got[1].Model != "claude" || got[1].Requests != 15 || got[1].Tokens != 232 {
		t.Errorf("second = %+v, want claude/15/232", got[1])
	}
}

func TestTopModelsByRequests_HonorsLimit(t *testing.T) {
	in := []*domain.UsageStats{
		{Model: "a", TotalRequests: 1},
		{Model: "b", TotalRequests: 2},
		{Model: "c", TotalRequests: 3},
		{Model: "d", TotalRequests: 4},
		{Model: "e", TotalRequests: 5},
	}
	got := TopModelsByRequests(in, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Top 3 by requests desc: e=5, d=4, c=3
	wantOrder := []string{"e", "d", "c"}
	for i, w := range wantOrder {
		if got[i].Model != w {
			t.Errorf("got[%d].Model = %q, want %q", i, got[i].Model, w)
		}
	}
}

func TestDailyCounts_Empty(t *testing.T) {
	got := DailyCounts(nil, time.UTC)
	if len(got) != 0 {
		t.Errorf("nil input → %v, want empty", got)
	}
	got = DailyCounts([]*domain.UsageStats{}, time.UTC)
	if len(got) != 0 {
		t.Errorf("empty input → %v, want empty", got)
	}
}

func TestDailyCounts_SkipsZeroRequestRows(t *testing.T) {
	// 一行 TotalRequests=0 不应该产生热力图条目(原 dashboard 在输出阶段 if count > 0 过滤掉)。
	day := time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)
	in := []*domain.UsageStats{
		{TimeBucket: day, TotalRequests: 0},
	}
	got := DailyCounts(in, time.UTC)
	if len(got) != 0 {
		t.Errorf("zero-request row → %v, want empty", got)
	}
}

func TestDailyCounts_AggregatesByDate(t *testing.T) {
	// 同一日期多条 stats(不同 provider/model 等维度)应累加。
	day := time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)
	in := []*domain.UsageStats{
		{TimeBucket: day, ProviderID: 1, TotalRequests: 5},
		{TimeBucket: day, ProviderID: 2, TotalRequests: 3},
		{TimeBucket: day.Add(2 * time.Hour), ProviderID: 1, TotalRequests: 7}, // 同一天不同小时
		{TimeBucket: day.AddDate(0, 0, 1), TotalRequests: 11},                 // 第二天
	}
	got := DailyCounts(in, time.UTC)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 days", len(got))
	}
	if got[0].Date != "2024-03-05" || got[0].Count != 15 {
		t.Errorf("day[0] = %+v, want {2024-03-05, 15}", got[0])
	}
	if got[1].Date != "2024-03-06" || got[1].Count != 11 {
		t.Errorf("day[1] = %+v, want {2024-03-06, 11}", got[1])
	}
}

func TestDailyCounts_SortedAscending(t *testing.T) {
	in := []*domain.UsageStats{
		{TimeBucket: time.Date(2024, 3, 7, 0, 0, 0, 0, time.UTC), TotalRequests: 1},
		{TimeBucket: time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC), TotalRequests: 1},
		{TimeBucket: time.Date(2024, 3, 6, 0, 0, 0, 0, time.UTC), TotalRequests: 1},
	}
	got := DailyCounts(in, time.UTC)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []string{"2024-03-05", "2024-03-06", "2024-03-07"}
	for i, w := range want {
		if got[i].Date != w {
			t.Errorf("got[%d].Date = %q, want %q (must sort ascending)", i, got[i].Date, w)
		}
	}
}

func TestDailyCounts_TimezoneAffectsDateBoundary(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("Asia/Shanghai not available")
	}
	// 2024-03-04 23:00 UTC = 2024-03-05 07:00 Shanghai → 同一桶在两个时区下日期不同。
	utcEvening := time.Date(2024, 3, 4, 23, 0, 0, 0, time.UTC)
	in := []*domain.UsageStats{
		{TimeBucket: utcEvening, TotalRequests: 5},
	}
	gotUTC := DailyCounts(in, time.UTC)
	if len(gotUTC) != 1 || gotUTC[0].Date != "2024-03-04" {
		t.Errorf("UTC: got %v, want one entry on 2024-03-04", gotUTC)
	}
	gotShanghai := DailyCounts(in, shanghai)
	if len(gotShanghai) != 1 || gotShanghai[0].Date != "2024-03-05" {
		t.Errorf("Shanghai: got %v, want one entry on 2024-03-05", gotShanghai)
	}
}

func TestHourlyTrend_Empty(t *testing.T) {
	start := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)
	got := HourlyTrend(nil, start, time.UTC)
	if len(got) != 24 {
		t.Fatalf("len = %d, want exactly 24 buckets even for empty input", len(got))
	}
	for i, p := range got {
		if p.Requests != 0 {
			t.Errorf("bucket[%d] = %d, want 0 (no data → all zeros)", i, p.Requests)
		}
	}
	// Order check: first bucket label should match `start` truncated to hour.
	if got[0].Hour != "10:00" {
		t.Errorf("got[0].Hour = %q, want 10:00", got[0].Hour)
	}
	if got[23].Hour != "09:00" {
		t.Errorf("got[23].Hour = %q, want 09:00 (24h later wraps)", got[23].Hour)
	}
}

func TestHourlyTrend_NonHourAlignedStartTruncates(t *testing.T) {
	// start = 10:30; hour-aligned start should be 10:00.
	start := time.Date(2024, 1, 17, 10, 30, 0, 0, time.UTC)
	got := HourlyTrend(nil, start, time.UTC)
	if got[0].Hour != "10:00" {
		t.Errorf("got[0].Hour = %q, want 10:00 (start truncates to hour)", got[0].Hour)
	}
}

func TestHourlyTrend_AccumulatesByHour(t *testing.T) {
	// hour-granularity stats: TimeBucket is hour-aligned (matches Query output).
	// Two rows in the same hour bucket — sharded by another dimension — should sum.
	start := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)
	in := []*domain.UsageStats{
		{TimeBucket: start, ProviderID: 1, TotalRequests: 5},               // "10:00"
		{TimeBucket: start, ProviderID: 2, TotalRequests: 3},               // "10:00" — same bucket, different provider
		{TimeBucket: start.Add(1 * time.Hour), TotalRequests: 7},           // "11:00"
		{TimeBucket: start.Add(23 * time.Hour), TotalRequests: 1},          // "09:00" (next day, last bucket)
	}
	got := HourlyTrend(in, start, time.UTC)
	if got[0].Requests != 8 {
		t.Errorf("got[0] = %d, want 8 (5+3 in same hour, different providers)", got[0].Requests)
	}
	if got[1].Requests != 7 {
		t.Errorf("got[1] = %d, want 7", got[1].Requests)
	}
	if got[23].Requests != 1 {
		t.Errorf("got[23] = %d, want 1", got[23].Requests)
	}
	if got[2].Requests != 0 {
		t.Errorf("got[2] = %d, want 0 (no data → zero-fill)", got[2].Requests)
	}
}

// TestHourlyTrend_DoesNotPolluteAcross24h 验证 review 指出的 collision bug 已修:
// 之前 helper 按 "HH:MM" 作 key,跨天的同 wall-clock 时间(如 1/17 10:00 和 1/18 10:00)
// 会被错误折叠到同一桶。现在按 hour-aligned UnixMilli 作 key,只有真正落在窗口
// [start, start+24h) 内的桶才匹配;窗口外的同 HH:MM stat 应被丢弃。
func TestHourlyTrend_DoesNotPolluteAcross24h(t *testing.T) {
	start := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)
	in := []*domain.UsageStats{
		// 同一 HH:MM=10:00,但日期是下一天 — 应被忽略,不应污染 start 那个桶。
		{TimeBucket: time.Date(2024, 1, 18, 10, 0, 0, 0, time.UTC), TotalRequests: 99},
		// 同一 HH:MM=10:00,但日期是上一天 — 也应被忽略。
		{TimeBucket: time.Date(2024, 1, 16, 10, 0, 0, 0, time.UTC), TotalRequests: 50},
	}
	got := HourlyTrend(in, start, time.UTC)
	// 所有 24 桶都应为 0(没有任何 stat 落在 [1/17 10:00, 1/18 10:00) 这个窗口里)。
	for i, p := range got {
		if p.Requests != 0 {
			t.Errorf("got[%d] = %d, want 0 (跨日的 HH:MM=10:00 stat 不能污染窗口内的桶)", i, p.Requests)
		}
	}
}

func TestHourlyTrend_NilLocationDefaultsUTC(t *testing.T) {
	start := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)
	// nil loc 不应 panic(原实现 t.In(nil) 会 panic)。
	got := HourlyTrend(nil, start, nil)
	if len(got) != 24 {
		t.Fatalf("nil loc: len = %d, want 24", len(got))
	}
	if got[0].Hour != "10:00" {
		t.Errorf("nil loc: got[0].Hour = %q, want 10:00 (默认 UTC)", got[0].Hour)
	}
}

func TestDailyCounts_NilLocationDefaultsUTC(t *testing.T) {
	// nil loc 不应 panic。
	in := []*domain.UsageStats{
		{TimeBucket: time.Date(2024, 3, 5, 12, 0, 0, 0, time.UTC), TotalRequests: 10},
	}
	got := DailyCounts(in, nil)
	if len(got) != 1 {
		t.Fatalf("nil loc: len = %d, want 1", len(got))
	}
	if got[0].Date != "2024-03-05" {
		t.Errorf("nil loc: got[0].Date = %q, want 2024-03-05 (默认 UTC)", got[0].Date)
	}
}

// TestHourlyTrend_FractionalHourOffsetTZAlignsToStorageGrid 锁住 R3 修复后的行为:
// 桶/key 的对齐方式跟 stats 存储用的 TruncateToGranularity(Hour, loc) 一致。
// Asia/Kathmandu(UTC+05:45)的存储网格是 absolute UTC hour(因为 Truncate 是 UTC-aligned),
// 在 Kathmandu 下渲染就是 ":45" 这种分钟。这是 storage-grid 真实就在这,helper 不应假装它在 ":00"。
func TestHourlyTrend_FractionalHourOffsetTZAlignsToStorageGrid(t *testing.T) {
	kathmandu, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		t.Skip("Asia/Kathmandu not available")
	}
	// 模拟一个已经存进 DB 的 hour-stat:它必然是 absolute-UTC-hour 对齐 (= Kathmandu ":45" wall-clock)。
	utcHour := time.Date(2024, 1, 17, 21, 0, 0, 0, time.UTC)
	in := []*domain.UsageStats{
		{TimeBucket: utcHour, TotalRequests: 7}, // 在 Kathmandu 显示为 02:45
	}
	// start 跟 stat 在同一时刻(用 UTC hour) — bucket[0] 应落在这里。
	got := HourlyTrend(in, utcHour, kathmandu)
	if got[0].Requests != 7 {
		t.Errorf("got[0].Requests = %d, want 7 (stat 跟 bucket[0] 在同一存储网格上)", got[0].Requests)
	}
	// 渲染出 ":45" 是真实的 storage-grid 偏移,不是 bug。
	if got[0].Hour != "02:45" {
		t.Errorf("got[0].Hour = %q, want 02:45 (Kathmandu 的 hour-bucket 网格落在 :45 分钟)", got[0].Hour)
	}
	if got[1].Hour != "03:45" {
		t.Errorf("got[1].Hour = %q, want 03:45 (下一桶 +1h)", got[1].Hour)
	}
}

// TestHourlyTrend_DSTFallBackNoCollision 验证 R3 review 指出的 DST fall-back collision bug 已修:
// 2024-11-03 在 America/New_York 时区:01:00 EDT (05:00 UTC) 和 01:00 EST (06:00 UTC) 是两个真实
// 不同的小时桶,中间相隔 1 小时。R2 版本的 truncateHourInLoc 会通过 time.Date(...,1,0,0,0,NY)
// 把两者折叠到同一个 UnixMilli (EDT 的那个),导致 chart 上 02:00 桶被错误地占用、01:00 桶被双倍。
//
// 新版用绝对 UnixMilli 作 key,加 1h 一次就走 1h 真实时间;两个 01:00 NY 桶 → 两个 distinct key,
// 数据正确分开;chart 上会有两个相邻 "01:00" 标签(这是 wall-clock 渲染的固有歧义,无法消除)。
func TestHourlyTrend_DSTFallBackNoCollision(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("America/New_York not available")
	}
	// 在 NY fall-back 那天(2024-11-03)从午夜开始一个 24h 窗口。
	// 这一天 NY 局部时间有 25 个小时(02:00 EDT 在 01:00 EST 之前再多 1 小时);窗口包含两个 "01:00"。
	start := time.Date(2024, 11, 3, 0, 0, 0, 0, ny)
	t1 := time.Date(2024, 11, 3, 5, 0, 0, 0, time.UTC) // 01:00 EDT
	t2 := time.Date(2024, 11, 3, 6, 0, 0, 0, time.UTC) // 01:00 EST
	in := []*domain.UsageStats{
		{TimeBucket: t1, TotalRequests: 11},
		{TimeBucket: t2, TotalRequests: 22},
	}
	got := HourlyTrend(in, start, ny)

	// 验证:t1 和 t2 都精确落到了自己的桶,没有合并、没有覆盖、也没有跑到 02:00 桶。
	var t1Slot, t2Slot int = -1, -1
	for i, p := range got {
		if p.Requests == 11 {
			t1Slot = i
		}
		if p.Requests == 22 {
			t2Slot = i
		}
	}
	if t1Slot < 0 || t2Slot < 0 {
		t.Fatalf("two DST hours collapsed; got = %+v", got)
	}
	if t1Slot == t2Slot {
		t.Fatalf("01:00 EDT 和 01:00 EST 落到了同一个 slot[%d] — bucket key collision 未修", t1Slot)
	}
	if t2Slot != t1Slot+1 {
		t.Errorf("两个 DST hour 应相邻 (相差 1 小时绝对时间); t1@%d t2@%d", t1Slot, t2Slot)
	}
	// 渲染上,这两个相邻 slot 都会显示 "01:00" — 这是 wall-clock 的固有歧义,锁住一下提醒未来读者。
	if got[t1Slot].Hour != "01:00" || got[t2Slot].Hour != "01:00" {
		t.Errorf("DST fall-back 渲染:两个相邻 slot 都该贴 01:00 标签; got %q / %q", got[t1Slot].Hour, got[t2Slot].Hour)
	}
	// 任何其他 slot 都不应混到这两个 stat 的请求量
	for i, p := range got {
		if i == t1Slot || i == t2Slot {
			continue
		}
		if p.Requests != 0 {
			t.Errorf("slot[%d] = %d, 应为 0 (DST stat 不应跨桶污染)", i, p.Requests)
		}
	}
}

func TestHourlyTrend_RespectsTimezone(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("Asia/Shanghai not available")
	}
	// start = 02:00 UTC = 10:00 Shanghai.
	// In Shanghai, the bucket label for that hour is "10:00".
	start := time.Date(2024, 1, 17, 2, 0, 0, 0, time.UTC)
	in := []*domain.UsageStats{
		{TimeBucket: start, TotalRequests: 42},
	}
	gotUTC := HourlyTrend(in, start, time.UTC)
	if gotUTC[0].Hour != "02:00" || gotUTC[0].Requests != 42 {
		t.Errorf("UTC[0] = %+v, want {02:00, 42}", gotUTC[0])
	}
	gotShanghai := HourlyTrend(in, start, shanghai)
	if gotShanghai[0].Hour != "10:00" || gotShanghai[0].Requests != 42 {
		t.Errorf("Shanghai[0] = %+v, want {10:00, 42}", gotShanghai[0])
	}
}

func TestTopModelsByRequests_TiesAreDeterministic(t *testing.T) {
	// All models tied on requests; expect alphabetical order so result is stable.
	in := []*domain.UsageStats{
		{Model: "zeta", TotalRequests: 5},
		{Model: "alpha", TotalRequests: 5},
		{Model: "mid", TotalRequests: 5},
	}
	got := TopModelsByRequests(in, 3)
	if got[0].Model != "alpha" || got[1].Model != "mid" || got[2].Model != "zeta" {
		t.Errorf("tie order = [%s, %s, %s], want [alpha, mid, zeta]",
			got[0].Model, got[1].Model, got[2].Model)
	}
}
