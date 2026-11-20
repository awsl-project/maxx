package privacy

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

func checkFailure(status int) *Rejection {
	return &Rejection{Code: "privacy_check_failed", Message: "Unable to inspect request privacy", HTTPStatusCode: status}
}
