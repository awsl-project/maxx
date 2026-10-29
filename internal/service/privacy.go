package service

import "github.com/awsl-project/maxx/internal/privacy"

// Privacy is shared with the executor so settings, previews and records use the
// same database-scoped runtime. The fallback supports services built in tests.
func (s *AdminService) Privacy() *privacy.Service {
	s.privacyOnce.Do(func() { s.privacyService = privacy.NewService(s.settingRepo) })
	return s.privacyService
}

// SetPrivacyService is called during startup, before serving requests.
func (s *AdminService) SetPrivacyService(p *privacy.Service) {
	s.privacyOnce.Do(func() { s.privacyService = p })
}
