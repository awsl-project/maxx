package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

type proxyBooleanSettingRepo struct {
	values []string
	errs   []error
	reads  int
}

func (r *proxyBooleanSettingRepo) Get(key string) (string, error) {
	r.reads++
	idx := r.reads - 1
	if idx < len(r.errs) && r.errs[idx] != nil {
		return "", r.errs[idx]
	}
	if idx < len(r.values) {
		return r.values[idx], nil
	}
	if len(r.values) > 0 {
		return r.values[len(r.values)-1], nil
	}
	return "", nil
}

func (r *proxyBooleanSettingRepo) Set(key, value string) error              { return nil }
func (r *proxyBooleanSettingRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }
func (r *proxyBooleanSettingRepo) Delete(key string) error                  { return nil }

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "bad request")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var payload map[string]map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload["error"]["message"] != "bad request" {
		t.Fatalf("payload = %v, want error message", payload)
	}
}

func TestWriteRateLimitError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeRateLimitError(rec, "API token concurrent request limit exceeded", 1)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}

	var payload map[string]map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload["error"]["type"] != "rate_limit_error" {
		t.Fatalf("payload = %v, want rate_limit_error", payload)
	}
}

func TestWriteProxyErrorPreservesStatusAndRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	until := time.Now().Add(2 * time.Second)
	writeProxyError(rec, &domain.ProxyError{
		Err:            domain.ErrUpstreamError,
		Message:        "upstream returned status 429",
		Retryable:      true,
		HTTPStatusCode: http.StatusTooManyRequests,
		CooldownUntil:  &until,
	})

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	got := rec.Header().Get("Retry-After")
	if got == "" {
		t.Fatal("expected Retry-After header")
	}
	sec, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("Retry-After = %q, parse error: %v", got, err)
	}
	if sec < 1 || sec > 2 {
		t.Fatalf("Retry-After = %d, want 1 or 2", sec)
	}
}

func TestWriteStreamRateLimitError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeStreamRateLimitError(rec, "API token concurrent request limit exceeded", 1)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if !strings.Contains(rec.Body.String(), `"type":"rate_limit_error"`) {
		t.Fatalf("stream body = %q, want rate_limit_error", rec.Body.String())
	}
}

func TestWriteStreamErrorPreservesStatusAndRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	until := time.Now().Add(2 * time.Second)
	writeStreamError(rec, &domain.ProxyError{
		Err:            domain.ErrUpstreamError,
		Message:        "upstream returned status 429",
		Retryable:      true,
		HTTPStatusCode: http.StatusTooManyRequests,
		CooldownUntil:  &until,
	})

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	got := rec.Header().Get("Retry-After")
	if got == "" {
		t.Fatal("expected Retry-After header")
	}
	sec, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("Retry-After = %q, parse error: %v", got, err)
	}
	if sec < 1 || sec > 2 {
		t.Fatalf("Retry-After = %d, want 1 or 2", sec)
	}
	if !strings.Contains(rec.Body.String(), `"type":"error"`) {
		t.Fatalf("stream body = %q, want error event", rec.Body.String())
	}
}

func TestProxyHandlerRejectsRequestsWhenKillSwitchEnabled(t *testing.T) {
	systemsettingcache.Invalidate(domain.SettingKeyProxyRequestsDisabled)
	repo := &proxyBooleanSettingRepo{values: []string{"true"}}
	h := NewProxyHandler(nil, nil, nil, repo, nil)

	req := httptest.NewRequest(http.MethodPost, "http://example.test/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[]}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), proxyRequestsDisabledMessage) {
		t.Fatalf("body = %q, want disabled message", rec.Body.String())
	}
}

func TestProxyKillSwitchDefaultsFalse(t *testing.T) {
	systemsettingcache.Invalidate(domain.SettingKeyProxyRequestsDisabled)
	repo := &proxyBooleanSettingRepo{}
	if systemsettingcache.GetBoolean(repo, domain.SettingKeyProxyRequestsDisabled) {
		t.Fatal("expected missing setting to default to false")
	}
}

func TestProxyKillSwitchCachesFreshValue(t *testing.T) {
	oldTTL := systemsettingcache.BooleanTTL
	systemsettingcache.BooleanTTL = time.Hour
	defer func() { systemsettingcache.BooleanTTL = oldTTL }()

	key := domain.SettingKeyProxyRequestsDisabled
	systemsettingcache.Invalidate(key)
	repo := &proxyBooleanSettingRepo{values: []string{"true"}}

	if !systemsettingcache.GetBoolean(repo, key) {
		t.Fatal("expected first read to return true")
	}
	if !systemsettingcache.GetBoolean(repo, key) {
		t.Fatal("expected cached read to return true")
	}
	if repo.reads != 1 {
		t.Fatalf("reads = %d, want 1", repo.reads)
	}
}

func TestProxyKillSwitchFallsBackToLastKnownValueOnReadError(t *testing.T) {
	oldTTL := systemsettingcache.BooleanTTL
	systemsettingcache.BooleanTTL = time.Nanosecond
	defer func() { systemsettingcache.BooleanTTL = oldTTL }()

	key := domain.SettingKeyProxyRequestsDisabled
	systemsettingcache.Invalidate(key)
	repo := &proxyBooleanSettingRepo{
		values: []string{"true"},
		errs:   []error{nil, errors.New("db temporarily unavailable")},
	}

	if !systemsettingcache.GetBoolean(repo, key) {
		t.Fatal("expected first read to return true")
	}
	time.Sleep(time.Millisecond)
	if !systemsettingcache.GetBoolean(repo, key) {
		t.Fatal("expected cached true value on refresh error")
	}
	if repo.reads != 2 {
		t.Fatalf("reads = %d, want 2", repo.reads)
	}
}

func TestUserPanelAPITokenProjectBindingGuard(t *testing.T) {
	userPanelToken := &domain.APIToken{
		Description: userPanelAPITokenDescription(123),
		ProjectID:   42,
	}
	regularToken := &domain.APIToken{
		Description: "regular token",
		ProjectID:   42,
	}

	if !isUserPanelAPIToken(userPanelToken) {
		t.Fatal("expected managed user panel token to be detected")
	}
	if canAPITokenUseProjectBinding(userPanelToken) {
		t.Fatal("user panel token must not use header/token/session project binding")
	}
	if got, ok := apiTokenProjectBinding(userPanelToken, 0); ok || got != 0 {
		t.Fatalf("user panel token project binding = (%d, %v), want (0, false)", got, ok)
	}
	if isUserPanelAPIToken(regularToken) {
		t.Fatal("regular token must not be treated as user panel token")
	}
	if !canAPITokenUseProjectBinding(regularToken) {
		t.Fatal("regular token should keep existing project binding behavior")
	}
	if got, ok := apiTokenProjectBinding(regularToken, 0); !ok || got != regularToken.ProjectID {
		t.Fatalf("regular token project binding = (%d, %v), want (%d, true)", got, ok, regularToken.ProjectID)
	}
	if got, ok := apiTokenProjectBinding(regularToken, 7); ok || got != 7 {
		t.Fatalf("existing project binding = (%d, %v), want (7, false)", got, ok)
	}
	if !canAPITokenUseProjectBinding(nil) {
		t.Fatal("nil token should keep unauthenticated/default project behavior")
	}
}

func TestResolveProxyProjectID(t *testing.T) {
	regularGlobalToken := &domain.APIToken{Description: "global token"}
	regularProjectToken := &domain.APIToken{Description: "project token", ProjectID: 42}
	userPanelToken := &domain.APIToken{
		Description: userPanelAPITokenDescription(123),
		ProjectID:   42,
	}

	tests := []struct {
		name        string
		header      string
		token       *domain.APIToken
		wantProject uint64
		wantErr     bool
	}{
		{name: "project proxy header with global token", header: "9", token: regularGlobalToken, wantProject: 9},
		{name: "token-bound project without header", token: regularProjectToken, wantProject: 42},
		{name: "project proxy header takes precedence", header: "9", token: regularProjectToken, wantProject: 9},
		{name: "invalid header falls back to token binding", header: "invalid", token: regularProjectToken, wantProject: 42},
		{name: "global token without project", token: regularGlobalToken},
		{name: "user panel token cannot select header project", header: "9", token: userPanelToken, wantErr: true},
		{name: "user panel token does not use token binding", token: userPanelToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.test/responses", nil)
			if tt.header != "" {
				req.Header.Set("X-Maxx-Project-ID", tt.header)
			}
			got, err := resolveProxyProjectID(req, tt.token)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantProject {
				t.Fatalf("project ID = %d, want %d", got, tt.wantProject)
			}
		})
	}
}
