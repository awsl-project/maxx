package privacy

import (
	"context"
)

// Settings is the existing system settings store, scoped to this Maxx instance.
type Settings interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Service owns the global policy. Tenant isolation is retained for learned
// values and history; requests never borrow another request's restore mapping.
type Service struct {
	settings Settings
	runtime  *Runtime
}

func NewService(settings Settings) *Service {
	return &Service{settings: settings, runtime: newRuntime()}
}

type Rejection struct {
	Code, Message  string
	HTTPStatusCode int
}

func (s *Service) Enabled() (bool, error) {
	if s == nil {
		return false, nil
	}
	cfg, err := s.Configuration()
	return cfg.Enabled, err
}

func (s *Service) Check(tenantID uint64, body []byte, contentType string) (bool, *Rejection) {
	exchange, rejection := s.Prepare(context.Background(), tenantID, body, body, contentType)
	if rejection == nil {
		rejection = exchange.ResolvePlan(nil)
	}
	return exchange != nil, rejection
}

func checkFailure(status int) *Rejection {
	return &Rejection{Code: "privacy_check_failed", Message: "Unable to inspect request privacy", HTTPStatusCode: status}
}
