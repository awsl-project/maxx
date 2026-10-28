package privacy

import (
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
)

// Projects is the tenant-scoped policy storage used by the privacy module.
type Projects interface {
	GetByID(tenantID, id uint64) (*domain.Project, error)
	Update(project *domain.Project) error
}

// Service owns project privacy policy and request decisions.
type Service struct{ projects Projects }

// NewService uses the same tenant-scoped project store as project settings.
func NewService(projects Projects) *Service { return &Service{projects: projects} }

// Rejection contains only safe information that can be returned to a client.
type Rejection struct {
	Code, Message  string
	HTTPStatusCode int
}

// Check returns the request-scoped policy and an optional rejection.
// Disabled projects bypass body parsing and detection entirely.
// Missing policy storage fails closed for selected projects.
func (s *Service) Check(tenantID, projectID uint64, body []byte, contentType string) (bool, *Rejection) {
	if projectID == 0 {
		return false, nil
	}
	if s == nil || s.projects == nil {
		return true, checkFailure(http.StatusServiceUnavailable)
	}
	project, err := s.projects.GetByID(tenantID, projectID)
	if err != nil || project == nil {
		return true, checkFailure(http.StatusServiceUnavailable)
	}
	if !project.BlockSecrets {
		return false, nil
	}
	kind, err := InspectRequest(body, contentType)
	if err != nil {
		return true, checkFailure(http.StatusBadRequest)
	}
	if kind == "" {
		return true, nil
	}
	return true, &Rejection{Code: "privacy_blocked", Message: "Request blocked: credential detected in message content (" + kind + ")", HTTPStatusCode: http.StatusBadRequest}
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
	if err := s.projects.Update(&updated); err != nil {
		return nil, err
	}
	return &updated, nil
}
