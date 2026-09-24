package service

import (
	"context"
	"sort"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

const (
	modelHealthWindow              = 24 * time.Hour
	modelHealthStaleAfter          = 10 * time.Minute
	modelHealthBucketCount         = 24
	modelHealthProbeTimeout        = 12 * time.Second
	modelHealthMaxProbesPerRequest = 20
)

type UserPanelModelHealthPoint struct {
	Status    domain.ModelHealthStatus `json:"status"`
	CheckedAt *time.Time               `json:"checkedAt,omitempty"`
	LatencyMs int64                    `json:"latencyMs,omitempty"`
	Error     string                   `json:"error,omitempty"`
}

type UserPanelModelHealthRow struct {
	Model        string                      `json:"model"`
	ClientType   domain.ClientType           `json:"clientType"`
	RouteID      uint64                      `json:"routeID"`
	ProviderID   uint64                      `json:"providerID"`
	ProviderName string                      `json:"providerName"`
	Points       []UserPanelModelHealthPoint `json:"points"`
	Current      UserPanelModelHealthPoint   `json:"current"`
}

func (s *AdminService) GetUserPanelModelHealthGrid(ctx context.Context, tenantID uint64, apiTokenID uint64, targets []domain.ModelHealthCheckTarget) ([]UserPanelModelHealthRow, error) {
	if s.modelHealthRepo == nil || tenantID == 0 || apiTokenID == 0 || len(targets) == 0 {
		return []UserPanelModelHealthRow{}, nil
	}
	targets = dedupeHealthTargets(targets)
	now := time.Now()
	latest, err := s.modelHealthRepo.LatestByTargets(tenantID, targets)
	if err != nil {
		return nil, err
	}
	probesStarted := 0
	for _, target := range targets {
		key := repository.ModelHealthTargetKey(target.Model, target.ClientType, target.RouteID, target.ProviderID)
		last := latest[key]
		if last != nil && now.Sub(last.CheckedAt) < modelHealthStaleAfter {
			continue
		}
		if probesStarted >= modelHealthMaxProbesPerRequest {
			continue
		}
		probesStarted++
		check := &domain.ModelHealthCheck{TenantID: tenantID, Model: target.Model, ClientType: target.ClientType, RouteID: target.RouteID, ProviderID: target.ProviderID, ProviderName: target.ProviderName, Status: domain.ModelHealthStatusUnknown, CheckedAt: now}
		if s.modelHealthChecker != nil {
			probeCtx, cancel := context.WithTimeout(ctx, modelHealthProbeTimeout)
			probe := s.modelHealthChecker.ProbeModelHealth(probeCtx, tenantID, apiTokenID, target)
			cancel()
			check.Status = probe.Status
			check.LatencyMs = probe.LatencyMs
			check.Error = probe.Error
			if check.Status == "" {
				check.Status = domain.ModelHealthStatusError
			}
		}
		if err := s.modelHealthRepo.Create(check); err != nil {
			return nil, err
		}
		latest[key] = check
	}
	since := now.Add(-modelHealthWindow)
	history, err := s.modelHealthRepo.ListByTargetsSince(tenantID, targets, since)
	if err != nil {
		return nil, err
	}
	return buildModelHealthRows(targets, history, latest, since, now), nil
}

func dedupeHealthTargets(targets []domain.ModelHealthCheckTarget) []domain.ModelHealthCheckTarget {
	seen := map[string]struct{}{}
	out := make([]domain.ModelHealthCheckTarget, 0, len(targets))
	for _, target := range targets {
		if target.Model == "" || target.RouteID == 0 || target.ProviderID == 0 {
			continue
		}
		if target.ClientType == "" {
			target.ClientType = domain.ClientTypeOpenAI
		}
		key := repository.ModelHealthTargetKey(target.Model, target.ClientType, target.RouteID, target.ProviderID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		if out[i].ClientType != out[j].ClientType {
			return out[i].ClientType < out[j].ClientType
		}
		if out[i].RouteID != out[j].RouteID {
			return out[i].RouteID < out[j].RouteID
		}
		return out[i].ProviderID < out[j].ProviderID
	})
	return out
}

func buildModelHealthRows(targets []domain.ModelHealthCheckTarget, checks []*domain.ModelHealthCheck, latest map[string]*domain.ModelHealthCheck, since time.Time, now time.Time) []UserPanelModelHealthRow {
	bucketSize := modelHealthWindow / modelHealthBucketCount
	byKeyBucket := map[string]map[int]*domain.ModelHealthCheck{}
	for _, check := range checks {
		key := repository.ModelHealthTargetKey(check.Model, check.ClientType, check.RouteID, check.ProviderID)
		idx := int(check.CheckedAt.Sub(since) / bucketSize)
		if idx < 0 {
			idx = 0
		}
		if idx >= modelHealthBucketCount {
			idx = modelHealthBucketCount - 1
		}
		if byKeyBucket[key] == nil {
			byKeyBucket[key] = map[int]*domain.ModelHealthCheck{}
		}
		if prev := byKeyBucket[key][idx]; prev == nil || check.CheckedAt.After(prev.CheckedAt) {
			byKeyBucket[key][idx] = check
		}
	}
	rows := make([]UserPanelModelHealthRow, 0, len(targets))
	for _, target := range targets {
		key := repository.ModelHealthTargetKey(target.Model, target.ClientType, target.RouteID, target.ProviderID)
		points := make([]UserPanelModelHealthPoint, modelHealthBucketCount)
		for i := range points {
			points[i] = UserPanelModelHealthPoint{Status: domain.ModelHealthStatusUnknown}
		}
		for idx, check := range byKeyBucket[key] {
			points[idx] = healthPoint(check)
		}
		current := UserPanelModelHealthPoint{Status: domain.ModelHealthStatusUnknown}
		if check := latest[key]; check != nil {
			current = healthPoint(check)
		}
		rows = append(rows, UserPanelModelHealthRow{Model: target.Model, ClientType: target.ClientType, RouteID: target.RouteID, ProviderID: target.ProviderID, ProviderName: target.ProviderName, Points: points, Current: current})
	}
	_ = now
	return rows
}

func healthPoint(check *domain.ModelHealthCheck) UserPanelModelHealthPoint {
	if check == nil {
		return UserPanelModelHealthPoint{Status: domain.ModelHealthStatusUnknown}
	}
	checkedAt := check.CheckedAt
	status := check.Status
	if status == "" {
		status = domain.ModelHealthStatusUnknown
	}
	return UserPanelModelHealthPoint{Status: status, CheckedAt: &checkedAt, LatencyMs: check.LatencyMs, Error: check.Error}
}
