package sqlite

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type ProxyUpstreamAttemptRepository struct {
	db *DB
}

func NewProxyUpstreamAttemptRepository(db *DB) *ProxyUpstreamAttemptRepository {
	return &ProxyUpstreamAttemptRepository{db: db}
}

func (r *ProxyUpstreamAttemptRepository) Create(a *domain.ProxyUpstreamAttempt) error {
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now

	model := r.toModel(a)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	a.ID = model.ID
	return nil
}

func (r *ProxyUpstreamAttemptRepository) Update(a *domain.ProxyUpstreamAttempt) error {
	a.UpdatedAt = time.Now()
	model := r.toModel(a)
	return r.db.gorm.Save(model).Error
}

func (r *ProxyUpstreamAttemptRepository) ListByProxyRequestID(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	var models []ProxyUpstreamAttempt
	if err := r.db.gorm.Where("proxy_request_id = ?", proxyRequestID).Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *ProxyUpstreamAttemptRepository) toModel(a *domain.ProxyUpstreamAttempt) *ProxyUpstreamAttempt {
	return &ProxyUpstreamAttempt{
		BaseModel: BaseModel{
			ID:        a.ID,
			CreatedAt: toTimestamp(a.CreatedAt),
			UpdatedAt: toTimestamp(a.UpdatedAt),
		},
		StartTime:         toTimestamp(a.StartTime),
		EndTime:           toTimestamp(a.EndTime),
		DurationMs:        a.Duration.Milliseconds(),
		Status:            a.Status,
		ProxyRequestID:    a.ProxyRequestID,
		IsStream:          boolToInt(a.IsStream),
		RequestModel:      a.RequestModel,
		MappedModel:       a.MappedModel,
		ResponseModel:     a.ResponseModel,
		RequestInfo:       LongText(toJSON(a.RequestInfo)),
		ResponseInfo:      LongText(toJSON(a.ResponseInfo)),
		RouteID:           a.RouteID,
		ProviderID:        a.ProviderID,
		InputTokenCount:   a.InputTokenCount,
		OutputTokenCount:  a.OutputTokenCount,
		CacheReadCount:    a.CacheReadCount,
		CacheWriteCount:   a.CacheWriteCount,
		Cache5mWriteCount: a.Cache5mWriteCount,
		Cache1hWriteCount: a.Cache1hWriteCount,
		Cost:              a.Cost,
	}
}

func (r *ProxyUpstreamAttemptRepository) toDomain(m *ProxyUpstreamAttempt) *domain.ProxyUpstreamAttempt {
	return &domain.ProxyUpstreamAttempt{
		ID:                m.ID,
		CreatedAt:         fromTimestamp(m.CreatedAt),
		UpdatedAt:         fromTimestamp(m.UpdatedAt),
		StartTime:         fromTimestamp(m.StartTime),
		EndTime:           fromTimestamp(m.EndTime),
		Duration:          time.Duration(m.DurationMs) * time.Millisecond,
		Status:            m.Status,
		ProxyRequestID:    m.ProxyRequestID,
		IsStream:          m.IsStream == 1,
		RequestModel:      m.RequestModel,
		MappedModel:       m.MappedModel,
		ResponseModel:     m.ResponseModel,
		RequestInfo:       fromJSON[*domain.RequestInfo](string(m.RequestInfo)),
		ResponseInfo:      fromJSON[*domain.ResponseInfo](string(m.ResponseInfo)),
		RouteID:           m.RouteID,
		ProviderID:        m.ProviderID,
		InputTokenCount:   m.InputTokenCount,
		OutputTokenCount:  m.OutputTokenCount,
		CacheReadCount:    m.CacheReadCount,
		CacheWriteCount:   m.CacheWriteCount,
		Cache5mWriteCount: m.Cache5mWriteCount,
		Cache1hWriteCount: m.Cache1hWriteCount,
		Cost:              m.Cost,
	}
}

func (r *ProxyUpstreamAttemptRepository) toDomainList(models []ProxyUpstreamAttempt) []*domain.ProxyUpstreamAttempt {
	attempts := make([]*domain.ProxyUpstreamAttempt, len(models))
	for i, m := range models {
		attempts[i] = r.toDomain(&m)
	}
	return attempts
}
