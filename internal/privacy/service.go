package privacy

import (
	"context"
	"errors"

	"github.com/awsl-project/maxx/internal/domain"
)

// Projects is the tenant-scoped policy storage used by the privacy module.
type Projects interface {
	GetByID(tenantID, id uint64) (*domain.Project, error)
	Update(project *domain.Project) error
}

// Service owns project privacy policy and request decisions.
type Service struct {
	projects Projects
	runtime  *Runtime
}

// NewService uses the same tenant-scoped project store as project settings.
func NewService(projects Projects) *Service {
	return &Service{projects: projects, runtime: newRuntime()}
}

// Rejection contains only safe information that can be returned to a client.
type Rejection struct {
	Code, Message  string
	HTTPStatusCode int
}

// Enabled reads only policy, including for early rejections with no request body.
// A missing project has no opt-in policy; storage failures remain errors.
func (s *Service) Enabled(tenantID, projectID uint64) (bool, error) {
	if projectID == 0 {
		return false, nil
	}
	if s == nil || s.projects == nil {
		return false, domain.ErrInvalidState
	}
	project, err := s.projects.GetByID(tenantID, projectID)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if project == nil {
		return false, domain.ErrInvalidState
	}
	return project.BlockSecrets, nil
}

// Check returns the request-scoped policy and an optional rejection.
// Disabled projects bypass body parsing and detection entirely.
func (s *Service) Check(tenantID, projectID uint64, body []byte, contentType string) (bool, *Rejection) {
	exchange, rejection := s.Prepare(context.Background(), tenantID, projectID, body, body, contentType)
	if rejection == nil {
		rejection = exchange.ResolvePlan(nil)
	}
	return exchange != nil, rejection
}

func checkFailure(status int) *Rejection {
	return &Rejection{Code: "privacy_check_failed", Message: "Unable to inspect request privacy", HTTPStatusCode: status}
}

// SetProjectProtection updates only privacy policy using current server data.
// Copying the project also preserves the cached repository's read-only objects.
func (s *Service) SetProjectProtection(tenantID, projectID uint64, enabled bool) (*domain.Project, error) {
	if s == nil || s.projects == nil {
		return nil, domain.ErrInvalidState
	}
	project, err := s.projects.GetByID(tenantID, projectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, domain.ErrNotFound
	}
	updated := *project
	updated.BlockSecrets = enabled
	if err := SyncLegacyProtection(&updated); err != nil {
		return nil, err
	}
	if err := s.projects.Update(&updated); err != nil {
		return nil, err
	}
	return &updated, nil
}
