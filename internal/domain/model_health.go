package domain

import "time"

type ModelHealthStatus string

const (
	ModelHealthStatusOK      ModelHealthStatus = "ok"
	ModelHealthStatusError   ModelHealthStatus = "error"
	ModelHealthStatusUnknown ModelHealthStatus = "unknown"
)

type ModelHealthCheckTarget struct {
	Model        string     `json:"model"`
	ClientType   ClientType `json:"clientType"`
	RouteID      uint64     `json:"routeID"`
	ProviderID   uint64     `json:"providerID"`
	ProviderName string     `json:"providerName"`
}

type ModelHealthProbeResult struct {
	Status    ModelHealthStatus
	LatencyMs int64
	Error     string
}

type ModelHealthCheck struct {
	ID           uint64
	TenantID     uint64
	Model        string
	ClientType   ClientType
	RouteID      uint64
	ProviderID   uint64
	ProviderName string
	Status       ModelHealthStatus
	LatencyMs    int64
	Error        string
	CheckedAt    time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
