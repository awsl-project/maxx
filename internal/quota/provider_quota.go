package quota

import (
	"fmt"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

const defaultWarningThreshold = 80

type UsageSummaryProvider interface {
	GetSummary(tenantID uint64, filter repository.UsageStatsFilter) (*domain.UsageStatsSummary, error)
}

type TimezoneSettingProvider interface {
	Get(key string) (string, error)
}

// EvaluateProviderQuota evaluates provider-level quota usage for the current period.
func EvaluateProviderQuota(
	provider *domain.Provider,
	tenantID uint64,
	usageRepo UsageSummaryProvider,
	settingRepo TimezoneSettingProvider,
	now time.Time,
) (*domain.ProviderQuotaStatus, error) {
	if provider == nil || provider.Config == nil || provider.Config.Quota == nil {
		return nil, nil
	}

	cfg := provider.Config.Quota
	period := normalizePeriod(cfg.Period)
	warningThreshold := normalizeWarningThreshold(cfg.WarningThresholdPercent)
	hasAnyLimit := cfg.RequestLimit > 0 || cfg.TokenLimit > 0 || cfg.CostLimit > 0

	loc, timezoneName := resolveLocation(settingRepo)
	periodStart, periodEnd := periodBounds(now, period, loc)

	status := &domain.ProviderQuotaStatus{
		Enabled:                 cfg.Enabled,
		Period:                  period,
		Timezone:                timezoneName,
		WarningThresholdPercent: warningThreshold,
		PeriodStart:             periodStart.UTC(),
		PeriodEnd:               periodEnd.UTC(),
		HasAnyLimit:             hasAnyLimit,
	}

	if !cfg.Enabled || !hasAnyLimit || usageRepo == nil {
		return status, nil
	}

	providerID := provider.ID
	filter := repository.UsageStatsFilter{
		Granularity: domain.GranularityMinute,
		StartTime:   ptrTime(periodStart.UTC()),
		EndTime:     ptrTime(periodEnd.UTC()),
		ProviderID:  &providerID,
	}

	summary, err := usageRepo.GetSummary(tenantID, filter)
	if err != nil {
		return status, fmt.Errorf("get provider quota summary: %w", err)
	}
	if summary == nil {
		summary = &domain.UsageStatsSummary{}
	}

	usedTokens := summary.TotalInputTokens + summary.TotalOutputTokens
	status.Requests = buildMetric(cfg.RequestLimit, summary.TotalRequests, warningThreshold)
	status.Tokens = buildMetric(cfg.TokenLimit, usedTokens, warningThreshold)
	status.Cost = buildMetric(cfg.CostLimit, summary.TotalCost, warningThreshold)

	if status.Requests != nil {
		status.Warning = status.Warning || status.Requests.Warning
		status.Exceeded = status.Exceeded || status.Requests.Exceeded
	}
	if status.Tokens != nil {
		status.Warning = status.Warning || status.Tokens.Warning
		status.Exceeded = status.Exceeded || status.Tokens.Exceeded
	}
	if status.Cost != nil {
		status.Warning = status.Warning || status.Cost.Warning
		status.Exceeded = status.Exceeded || status.Cost.Exceeded
	}

	return status, nil
}

func buildMetric(limit uint64, used uint64, warningThreshold int) *domain.ProviderQuotaMetricStatus {
	if limit == 0 {
		return nil
	}

	remaining := uint64(0)
	if used < limit {
		remaining = limit - used
	}

	usagePercent := float64(used) / float64(limit) * 100
	warning := usagePercent >= float64(warningThreshold)
	exceeded := used >= limit

	return &domain.ProviderQuotaMetricStatus{
		Limit:        limit,
		Used:         used,
		Remaining:    remaining,
		UsagePercent: usagePercent,
		Warning:      warning,
		Exceeded:     exceeded,
	}
}

func normalizePeriod(period domain.ProviderQuotaPeriod) domain.ProviderQuotaPeriod {
	switch period {
	case domain.ProviderQuotaPeriodWeek, domain.ProviderQuotaPeriodMonth:
		return period
	default:
		return domain.ProviderQuotaPeriodDay
	}
}

func normalizeWarningThreshold(v int) int {
	if v <= 0 || v > 100 {
		return defaultWarningThreshold
	}
	return v
}

func resolveLocation(settingRepo TimezoneSettingProvider) (*time.Location, string) {
	if settingRepo == nil {
		return time.UTC, "UTC"
	}

	name, err := settingRepo.Get(domain.SettingKeyTimezone)
	if err != nil {
		return time.UTC, "UTC"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return time.UTC, "UTC"
	}

	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC, "UTC"
	}
	return loc, name
}

func periodBounds(now time.Time, period domain.ProviderQuotaPeriod, loc *time.Location) (time.Time, time.Time) {
	localNow := now.In(loc)
	year, month, day := localNow.Date()
	startOfDay := time.Date(year, month, day, 0, 0, 0, 0, loc)

	switch period {
	case domain.ProviderQuotaPeriodWeek:
		weekday := int(startOfDay.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := startOfDay.AddDate(0, 0, -(weekday - 1))
		return start, start.AddDate(0, 0, 7)
	case domain.ProviderQuotaPeriodMonth:
		start := time.Date(year, month, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 1, 0)
	default:
		return startOfDay, startOfDay.AddDate(0, 0, 1)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
