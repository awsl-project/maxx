package sqlite

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// TestQueryDashboardHistoricalDays_GroupsByDimensions 锁住 PR573 R2 修复的"GROUP BY 下推"合约:
// 多维度行(不同 route_id / project_id / api_token_id / client_type)在同一天 / 同 provider /
// 同 model 下应该被 SQL 层聚合成 1 行返回,而不是 N 行;否则 dashboard query1 又会退化回
// 之前那种"高基数下笛卡尔积爆炸"的状态(Codex + Claude 在 PR573 R1 review 中都标过 must-fix)。
//
// 这是 dashboard 路径的 invariant,不该依赖人工 code review 来防回归。
func TestQueryDashboardHistoricalDays_GroupsByDimensions(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := NewUsageStatsRepository(db)

	// 同一天、同 provider、同 model,但 (route, project, token, client_type) 各异。
	// 8 行原始数据,GROUP BY (time_bucket, provider_id, model) 应该折成 1 行。
	day := time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)
	rows := []*UsageStats{
		{TenantID: 1, TimeBucket: toTimestamp(day), Granularity: string(domain.GranularityDay),
			RouteID: 1, ProjectID: 1, APITokenID: 1, ClientType: "claude", ProviderID: 10, Model: "claude-3-opus",
			TotalRequests: 5, SuccessfulRequests: 5, InputTokens: 100, OutputTokens: 50, Cost: 1000},
		{TenantID: 1, TimeBucket: toTimestamp(day), Granularity: string(domain.GranularityDay),
			RouteID: 2, ProjectID: 1, APITokenID: 1, ClientType: "claude", ProviderID: 10, Model: "claude-3-opus",
			TotalRequests: 3, SuccessfulRequests: 3, InputTokens: 80, OutputTokens: 40, Cost: 800},
		{TenantID: 1, TimeBucket: toTimestamp(day), Granularity: string(domain.GranularityDay),
			RouteID: 1, ProjectID: 2, APITokenID: 1, ClientType: "claude", ProviderID: 10, Model: "claude-3-opus",
			TotalRequests: 2, SuccessfulRequests: 2, InputTokens: 60, OutputTokens: 30, Cost: 600},
		{TenantID: 1, TimeBucket: toTimestamp(day), Granularity: string(domain.GranularityDay),
			RouteID: 1, ProjectID: 1, APITokenID: 2, ClientType: "claude", ProviderID: 10, Model: "claude-3-opus",
			TotalRequests: 4, SuccessfulRequests: 3, InputTokens: 90, OutputTokens: 45, Cost: 900},
		{TenantID: 1, TimeBucket: toTimestamp(day), Granularity: string(domain.GranularityDay),
			RouteID: 1, ProjectID: 1, APITokenID: 1, ClientType: "codex", ProviderID: 10, Model: "claude-3-opus",
			TotalRequests: 1, SuccessfulRequests: 1, InputTokens: 20, OutputTokens: 10, Cost: 200},
		// 不同 provider — 应单独返回一行
		{TenantID: 1, TimeBucket: toTimestamp(day), Granularity: string(domain.GranularityDay),
			RouteID: 1, ProjectID: 1, APITokenID: 1, ClientType: "claude", ProviderID: 20, Model: "claude-3-opus",
			TotalRequests: 7, SuccessfulRequests: 7, InputTokens: 200, OutputTokens: 100, Cost: 2000},
		// 不同 model — 应单独返回一行
		{TenantID: 1, TimeBucket: toTimestamp(day), Granularity: string(domain.GranularityDay),
			RouteID: 1, ProjectID: 1, APITokenID: 1, ClientType: "claude", ProviderID: 10, Model: "claude-3-sonnet",
			TotalRequests: 6, SuccessfulRequests: 6, InputTokens: 150, OutputTokens: 75, Cost: 1500},
		// 不同租户 — 应被 tenant 过滤掉
		{TenantID: 999, TimeBucket: toTimestamp(day), Granularity: string(domain.GranularityDay),
			RouteID: 1, ProjectID: 1, APITokenID: 1, ClientType: "claude", ProviderID: 10, Model: "claude-3-opus",
			TotalRequests: 100, SuccessfulRequests: 100, InputTokens: 9999, OutputTokens: 9999, Cost: 9999},
	}
	if err := db.gorm.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := repo.queryDashboardHistoricalDays(1, day, day.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("queryDashboardHistoricalDays: %v", err)
	}

	// 预期:对于 tenant=1,应返回 3 行,对应 (time, provider, model) 的 3 个组合:
	//  - (day, 10, claude-3-opus)
	//  - (day, 20, claude-3-opus)
	//  - (day, 10, claude-3-sonnet)
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3 (GROUP BY (time,provider,model) collapse 应生效)", len(got))
	}

	// 找到 (provider=10, model=claude-3-opus) 这条,它应该是前 5 行的累加值。
	var opusRow *domain.UsageStats
	for _, r := range got {
		if r.ProviderID == 10 && r.Model == "claude-3-opus" {
			opusRow = r
			break
		}
	}
	if opusRow == nil {
		t.Fatal("missing (provider=10, model=claude-3-opus) aggregated row")
	}
	const wantOpusRequests uint64 = 5 + 3 + 2 + 4 + 1
	const wantOpusInput uint64 = 100 + 80 + 60 + 90 + 20
	const wantOpusCost uint64 = 1000 + 800 + 600 + 900 + 200
	const wantOpusSuccess uint64 = 5 + 3 + 2 + 3 + 1
	if opusRow.TotalRequests != wantOpusRequests {
		t.Errorf("opus TotalRequests = %d, want %d (5 dimensions 必须 SUM 到一起)", opusRow.TotalRequests, wantOpusRequests)
	}
	if opusRow.SuccessfulRequests != wantOpusSuccess {
		t.Errorf("opus SuccessfulRequests = %d, want %d (GroupByProvider 需要这个字段算 SuccessRate)", opusRow.SuccessfulRequests, wantOpusSuccess)
	}
	if opusRow.InputTokens != wantOpusInput {
		t.Errorf("opus InputTokens = %d, want %d", opusRow.InputTokens, wantOpusInput)
	}
	if opusRow.Cost != wantOpusCost {
		t.Errorf("opus Cost = %d, want %d", opusRow.Cost, wantOpusCost)
	}

	// 验证其他维度未被泄漏到结果里(都应是 0,因为 SELECT 没投影它们)。
	if opusRow.RouteID != 0 || opusRow.APITokenID != 0 || opusRow.ProjectID != 0 || opusRow.ClientType != "" {
		t.Errorf("opus row leaked dimensions: %+v (查询不应填这些字段)", opusRow)
	}
}

// TestQueryDashboardHistoricalDays_ExclusiveEnd 锁住时间窗口是 [startInclusive, endExclusive):
// 落在 endExclusive 那一秒的桶不应返回(原 dashboard raw SQL 用 `<` 排除今天的桶,
// PR573 R1 在 Go 层用 FilterByTimeRange 兜底; R2 修复后这条 invariant 由 SQL 直接保证)。
func TestQueryDashboardHistoricalDays_ExclusiveEnd(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := NewUsageStatsRepository(db)

	start := time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 3, 6, 0, 0, 0, 0, time.UTC)

	rows := []*UsageStats{
		{TenantID: 1, TimeBucket: toTimestamp(start), Granularity: string(domain.GranularityDay),
			ProviderID: 1, Model: "m", TotalRequests: 1},
		// 落在 endExclusive 那一秒 — 应被排除
		{TenantID: 1, TimeBucket: toTimestamp(end), Granularity: string(domain.GranularityDay),
			ProviderID: 1, Model: "m", TotalRequests: 99},
	}
	if err := db.gorm.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := repo.queryDashboardHistoricalDays(1, start, end)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1 (endExclusive 那一秒不应被纳入)", len(got))
	}
	if got[0].TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1 (端点排他: end 那行的 99 不该被聚合进来)", got[0].TotalRequests)
	}
}

// TestQueryDashboardHistoricalDays_EmptyResult 锁住空结果不 panic / 不会返回 nil 之外的鬼东西。
func TestQueryDashboardHistoricalDays_EmptyResult(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := NewUsageStatsRepository(db)
	start := time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)
	got, err := repo.queryDashboardHistoricalDays(1, start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (DB 空)", len(got))
	}
}
