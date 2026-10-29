package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/awsl-project/maxx/internal/domain"
	"net/http"

	"github.com/awsl-project/maxx/internal/privacy/redact"
)

type Preview struct {
	Body    string       `json:"body"`
	Hits    []redact.Hit `json:"hits"`
	Blocked bool         `json:"blocked"`
}

// Preview applies an editor draft without changing policy, recording history,
// learning originals, or touching the tenant's placeholder identity/cache.
func (s *Service) Preview(ctx context.Context, config Config, body []byte) (Preview, *Rejection) {
	cfg, err := ValidateConfig(config)
	if err != nil {
		return Preview{}, checkFailure(http.StatusBadRequest)
	}
	runtime := newRuntime()
	runtime.find = s.runtime.find
	e := &Exchange{Enabled: true, Config: cfg, runtime: runtime, ctx: ctx, contentType: "application/json", rawBody: body, rawOriginal: body, Body: body, OriginalBody: body, pairs: map[string]string{}, matched: map[string]bool{}}
	rejection := e.evaluate()
	if rejection != nil && rejection.Code != "privacy_blocked" {
		return Preview{}, rejection
	}
	return Preview{Body: string(e.Body), Hits: append([]redact.Hit{}, e.Hits...), Blocked: rejection != nil}, nil
}

func (s *Service) EarlyArchiveFilter(tenant uint64) func(string) string {
	if s == nil {
		return nil
	}
	cfg, err := s.Configuration()
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return func(text string) string {
			if text == "" {
				return text
			}
			return "[REDACTED: privacy policy unavailable]"
		}
	}
	if !cfg.Enabled {
		return nil
	}
	// No request body is read for early rejections. Model-only findings cannot
	// be recovered safely here; omit inspected fields when the model was asked.
	e := &Exchange{Enabled: true, Config: cfg, runtime: s.runtime, key: scopeKey{tenant}, evaluated: true, failed: cfg.Model.Enabled, matched: map[string]bool{}}
	return e.Archive
}

// TextRequest is the preview convenience format, using the normal JSON walker.
func TextRequest(text string) []byte {
	b, _ := json.Marshal(map[string]string{"input": text})
	return b
}
