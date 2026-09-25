package executor

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type recordingProxyRequestRepo struct {
	created []*domain.ProxyRequest
	updated []*domain.ProxyRequest
}

func (r *recordingProxyRequestRepo) Create(req *domain.ProxyRequest) error {
	snap := *req
	r.created = append(r.created, &snap)
	return nil
}

func (r *recordingProxyRequestRepo) Update(req *domain.ProxyRequest) error {
	snap := *req
	r.updated = append(r.updated, &snap)
	return nil
}

func (r *recordingProxyRequestRepo) GetByID(uint64, uint64) (*domain.ProxyRequest, error) {
	return nil, nil
}

func (r *recordingProxyRequestRepo) List(uint64, int, int) ([]*domain.ProxyRequest, error) {
	return nil, nil
}

func (r *recordingProxyRequestRepo) ListCursor(uint64, int, uint64, uint64, *repository.ProxyRequestFilter) ([]*domain.ProxyRequest, error) {
	return nil, nil
}

func (r *recordingProxyRequestRepo) ListActive(uint64) ([]*domain.ProxyRequest, error) {
	return nil, nil
}

func (r *recordingProxyRequestRepo) Count(uint64) (int64, error) { return 0, nil }

func (r *recordingProxyRequestRepo) CountWithFilter(uint64, *repository.ProxyRequestFilter) (int64, error) {
	return 0, nil
}

func (r *recordingProxyRequestRepo) GetErrorStats(uint64, *repository.ProxyRequestFilter) (*repository.ProxyRequestErrorStats, error) {
	return nil, nil
}

func (r *recordingProxyRequestRepo) CountFailedWithFilter(uint64, *repository.ProxyRequestFilter) (int64, error) {
	return 0, nil
}

func (r *recordingProxyRequestRepo) DeleteFailedWithFilter(uint64, *repository.ProxyRequestFilter) (int64, int64, error) {
	return 0, 0, nil
}

func (r *recordingProxyRequestRepo) UpdateProjectIDBySessionID(uint64, string, uint64) (int64, error) {
	return 0, nil
}

func (r *recordingProxyRequestRepo) MarkStaleAsFailed([]string) (int64, error) {
	return 0, nil
}

func (r *recordingProxyRequestRepo) FixFailedRequestsWithoutEndTime() (int64, error) {
	return 0, nil
}

func (r *recordingProxyRequestRepo) DeleteOlderThan(time.Time) (int64, error) {
	return 0, nil
}

func (r *recordingProxyRequestRepo) HasRecentRequests(time.Time) (bool, error) {
	return false, nil
}

func (r *recordingProxyRequestRepo) GetProjectUsageSummaries(uint64, time.Time, ...uint64) (map[uint64]domain.ProjectUsageSummary, error) {
	return nil, nil
}

func (r *recordingProxyRequestRepo) UpdateCost(uint64, uint64) error { return nil }

func (r *recordingProxyRequestRepo) UpdateCostAtomically(uint64, uint64, map[uint64]domain.AttemptCostUpdate) error {
	return nil
}

func (r *recordingProxyRequestRepo) RecalculateCostsFromAttempts() (int64, error) {
	return 0, nil
}

func (r *recordingProxyRequestRepo) RecalculateCostsFromAttemptsWithProgress(chan<- domain.Progress) (int64, error) {
	return 0, nil
}

func (r *recordingProxyRequestRepo) ClearDetailOlderThan(time.Time, []string) (int64, error) {
	return 0, nil
}

type stubExecutorSettingsRepo struct {
	values map[string]string
}

func (r *stubExecutorSettingsRepo) Get(key string) (string, error) {
	if r.values == nil {
		return "", nil
	}
	return r.values[key], nil
}

func (r *stubExecutorSettingsRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *stubExecutorSettingsRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }
func (r *stubExecutorSettingsRepo) Delete(key string) error {
	delete(r.values, key)
	return nil
}

type stubModelMappingRepo struct {
	mappings []*domain.ModelMapping
}

func (r *stubModelMappingRepo) Create(*domain.ModelMapping) error { return nil }
func (r *stubModelMappingRepo) Update(*domain.ModelMapping) error { return nil }
func (r *stubModelMappingRepo) Delete(uint64, uint64) error       { return nil }
func (r *stubModelMappingRepo) GetByID(uint64, uint64) (*domain.ModelMapping, error) {
	return nil, nil
}
func (r *stubModelMappingRepo) List(uint64) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *stubModelMappingRepo) ListEnabled(uint64) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *stubModelMappingRepo) ListByClientType(uint64, domain.ClientType) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *stubModelMappingRepo) ListByQuery(uint64, *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	return r.mappings, nil
}
func (r *stubModelMappingRepo) Reorder(uint64, domain.ModelMappingReorderRequest) error {
	return nil
}
func (r *stubModelMappingRepo) Count(uint64) (int, error) { return 0, nil }
func (r *stubModelMappingRepo) DeleteAll(uint64) error    { return nil }
func (r *stubModelMappingRepo) ClearAll(uint64) error     { return nil }
func (r *stubModelMappingRepo) SeedDefaults(uint64) error { return nil }
