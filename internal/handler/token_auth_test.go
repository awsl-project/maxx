package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

type tokenAuthTestSettingRepo struct{}

func (tokenAuthTestSettingRepo) Get(key string) (string, error) {
	switch key {
	case SettingKeyProxyTokenAuthEnabled:
		return "true", nil
	case SettingKeyAPITokenConcurrentLimit:
		return "5", nil
	default:
		return "", nil
	}
}

func (tokenAuthTestSettingRepo) Set(key, value string) error              { return nil }
func (tokenAuthTestSettingRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }
func (tokenAuthTestSettingRepo) Delete(key string) error                  { return nil }

type tokenAuthUpdateCall struct {
	tenantID   uint64
	id         uint64
	lastIP     string
	lastSeenAt time.Time
}

type tokenAuthTestRepo struct {
	token    *domain.APIToken
	updates  chan tokenAuthUpdateCall
	createAt time.Time
}

func newTokenAuthTestRepo() *tokenAuthTestRepo {
	return &tokenAuthTestRepo{
		updates: make(chan tokenAuthUpdateCall, 4),
	}
}

func (r *tokenAuthTestRepo) Create(token *domain.APIToken) error {
	if token.ID == 0 {
		token.ID = 1
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now()
	}
	if token.UpdatedAt.IsZero() {
		token.UpdatedAt = token.CreatedAt
	}
	clone := *token
	r.token = &clone
	return nil
}

func (r *tokenAuthTestRepo) Update(token *domain.APIToken) error {
	clone := *token
	r.token = &clone
	return nil
}

func (r *tokenAuthTestRepo) Delete(tenantID uint64, id uint64) error { return nil }

func (r *tokenAuthTestRepo) DeleteExpired(tenantID uint64, now time.Time, inactiveExpiry time.Duration) ([]*domain.APIToken, error) {
	return []*domain.APIToken{}, nil
}

func (r *tokenAuthTestRepo) GetByID(tenantID uint64, id uint64) (*domain.APIToken, error) {
	if r.token == nil || r.token.ID != id {
		return nil, domain.ErrNotFound
	}
	clone := *r.token
	return &clone, nil
}

func (r *tokenAuthTestRepo) GetByToken(tenantID uint64, token string) (*domain.APIToken, error) {
	if r.token == nil || r.token.Token != token {
		return nil, domain.ErrNotFound
	}
	clone := *r.token
	return &clone, nil
}

func (r *tokenAuthTestRepo) List(tenantID uint64) ([]*domain.APIToken, error) {
	if r.token == nil {
		return nil, nil
	}
	clone := *r.token
	return []*domain.APIToken{&clone}, nil
}

func (r *tokenAuthTestRepo) UpdateLastSeen(tenantID uint64, id uint64, lastIP string, lastSeenAt time.Time) error {
	r.updates <- tokenAuthUpdateCall{tenantID: tenantID, id: id, lastIP: lastIP, lastSeenAt: lastSeenAt}
	if r.token != nil && r.token.ID == id {
		r.token.UseCount++
		r.token.LastUsedAt = &lastSeenAt
		if lastIP != "" {
			r.token.LastIP = lastIP
			r.token.LastIPAt = &lastSeenAt
		}
	}
	return nil
}

func (r *tokenAuthTestRepo) AddQuotaBalance(tenantID uint64, ids []uint64, amount uint64) (int64, error) {
	return 0, nil
}

func (r *tokenAuthTestRepo) DeductQuotaBalanceToZero(tenantID uint64, id uint64, amount uint64) error {
	return nil
}

func TestTokenAuthValidateRequestUpdatesLastSeenWithClientIP(t *testing.T) {
	repo := newTokenAuthTestRepo()
	cachedRepo := cached.NewAPITokenRepository(repo)
	token := &domain.APIToken{
		TenantID:    domain.DefaultTenantID,
		Token:       "maxx_test_token_123",
		TokenPrefix: "maxx_test...",
		Name:        "test-token",
		IsEnabled:   true,
	}
	if err := cachedRepo.Create(token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	middleware := NewTokenAuthMiddleware(cachedRepo, tokenAuthTestSettingRepo{})
	req := httptest.NewRequest("POST", "http://example.test/v1/chat/completions", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 127.0.0.1")

	validated, err := middleware.ValidateRequest(req, domain.ClientTypeOpenAI)
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if validated == nil {
		t.Fatal("ValidateRequest() returned nil token")
	}

	select {
	case update := <-repo.updates:
		if update.tenantID != token.TenantID {
			t.Fatalf("tenantID = %d, want %d", update.tenantID, token.TenantID)
		}
		if update.id != token.ID {
			t.Fatalf("id = %d, want %d", update.id, token.ID)
		}
		if update.lastIP != "203.0.113.9" {
			t.Fatalf("lastIP = %q, want %q", update.lastIP, "203.0.113.9")
		}
		if update.lastSeenAt.IsZero() {
			t.Fatal("lastSeenAt should be set")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for UpdateLastSeen call")
	}
}

func TestTokenAuthWrapModelListRequiresTokenWhenEnabled(t *testing.T) {
	repo := newTokenAuthTestRepo()
	cachedRepo := cached.NewAPITokenRepository(repo)
	apiToken := &domain.APIToken{
		TenantID:    7,
		Token:       "maxx_models_token_123",
		TokenPrefix: "maxx_models...",
		Name:        "models-token",
		IsEnabled:   true,
	}
	if err := cachedRepo.Create(apiToken); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	middleware := NewTokenAuthMiddleware(cachedRepo, tokenAuthTestSettingRepo{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := maxxctx.GetTenantID(r.Context()); got != 7 {
			t.Fatalf("tenant id = %d, want 7", got)
		}
		if got := maxxctx.GetAPITokenID(r.Context()); got != apiToken.ID {
			t.Fatalf("api token id = %d, want %d", got, apiToken.ID)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	})
	handler := middleware.WrapModelList(next)

	missingReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusUnauthorized {
		t.Fatalf("missing-token status = %d, want 401", missingRec.Code)
	}

	badReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	badReq.Header.Set("Authorization", "Bearer maxx_wrong")
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad-token status = %d, want 401", badRec.Code)
	}

	validReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	validReq.Header.Set("Authorization", "Bearer "+apiToken.Token)
	validRec := httptest.NewRecorder()
	handler.ServeHTTP(validRec, validReq)
	if validRec.Code != http.StatusOK {
		t.Fatalf("valid-token status = %d, want 200", validRec.Code)
	}
}

func TestTokenAuthWrapModelListAcceptsGeminiKeyHeader(t *testing.T) {
	repo := newTokenAuthTestRepo()
	cachedRepo := cached.NewAPITokenRepository(repo)
	apiToken := &domain.APIToken{
		TenantID:    domain.DefaultTenantID,
		Token:       "maxx_models_gemini_token",
		TokenPrefix: "maxx_models...",
		Name:        "gemini-models-token",
		IsEnabled:   true,
	}
	if err := cachedRepo.Create(apiToken); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	middleware := NewTokenAuthMiddleware(cachedRepo, tokenAuthTestSettingRepo{})
	handler := middleware.WrapModelList(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	req.Header.Set("x-goog-api-key", apiToken.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Gemini key header status = %d, want 200", rec.Code)
	}
}

func TestTokenAuthInactiveExpiryRejectsTokenLastUsedFifteenDaysAgo(t *testing.T) {
	repo := newTokenAuthTestRepo()
	cachedRepo := cached.NewAPITokenRepository(repo)
	lastUsedAt := time.Now().Add(-15 * 24 * time.Hour)
	token := &domain.APIToken{
		TenantID:    domain.DefaultTenantID,
		Token:       "maxx_test_token_inactive",
		TokenPrefix: "maxx_test...",
		Name:        "inactive-token",
		IsEnabled:   true,
		LastUsedAt:  &lastUsedAt,
	}
	if err := cachedRepo.Create(token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	middleware := NewTokenAuthMiddleware(cachedRepo, tokenAuthTestSettingRepo{})
	req := httptest.NewRequest("POST", "http://example.test/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+token.Token)

	validated, err := middleware.ValidateRequest(req, domain.ClientTypeOpenAI)
	if err != ErrTokenExpired {
		t.Fatalf("ValidateRequest() error = %v, want %v", err, ErrTokenExpired)
	}
	if validated != nil {
		t.Fatalf("ValidateRequest() token = %#v, want nil", validated)
	}

	select {
	case update := <-repo.updates:
		t.Fatalf("expired token should not update last seen, got %#v", update)
	default:
	}
}

func TestTokenAuthInactiveExpiryIgnoresMissingLastUsedAt(t *testing.T) {
	repo := newTokenAuthTestRepo()
	cachedRepo := cached.NewAPITokenRepository(repo)
	token := &domain.APIToken{
		TenantID:    domain.DefaultTenantID,
		Token:       "maxx_test_token_no_last_used",
		TokenPrefix: "maxx_test...",
		Name:        "no-last-used-token",
		IsEnabled:   true,
	}
	if err := cachedRepo.Create(token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	middleware := NewTokenAuthMiddleware(cachedRepo, tokenAuthTestSettingRepo{})
	req := httptest.NewRequest("POST", "http://example.test/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+token.Token)

	validated, err := middleware.ValidateRequest(req, domain.ClientTypeOpenAI)
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if validated == nil || validated.Token != token.Token {
		t.Fatalf("ValidateRequest() token = %#v, want %q", validated, token.Token)
	}

	select {
	case update := <-repo.updates:
		if update.lastSeenAt.IsZero() {
			t.Fatal("lastSeenAt should be set")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for UpdateLastSeen call")
	}
}

func TestInactiveAPITokenExpiredBoundary(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		lastUsedAt *time.Time
		want       bool
	}{
		{name: "missing last used", want: false},
		{name: "exactly ten days", lastUsedAt: ptrTime(now.Add(-domain.APITokenInactiveExpiry)), want: false},
		{name: "over ten days", lastUsedAt: ptrTime(now.Add(-domain.APITokenInactiveExpiry - time.Nanosecond)), want: true},
		{name: "fifteen days", lastUsedAt: ptrTime(now.Add(-15 * 24 * time.Hour)), want: true},
		{name: "within ten days", lastUsedAt: ptrTime(now.Add(-domain.APITokenInactiveExpiry + time.Nanosecond)), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInactiveAPITokenExpired(&domain.APIToken{LastUsedAt: tt.lastUsedAt}, now)
			if got != tt.want {
				t.Fatalf("isInactiveAPITokenExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestTokenAuthConcurrentLimitDefaultsToFive(t *testing.T) {
	repo := newTokenAuthTestRepo()
	cachedRepo := cached.NewAPITokenRepository(repo)
	middleware := NewTokenAuthMiddleware(cachedRepo, tokenAuthTestSettingRepo{})
	token := &domain.APIToken{ID: 42, Token: "maxx_test_token_456", IsEnabled: true}

	for i := 0; i < DefaultAPITokenConcurrentLimit; i++ {
		if err := middleware.AcquireConcurrency(token); err != nil {
			t.Fatalf("AcquireConcurrency() attempt %d error = %v", i+1, err)
		}
	}

	if err := middleware.AcquireConcurrency(token); err != ErrTokenConcurrentLimit {
		t.Fatalf("AcquireConcurrency() over limit error = %v, want %v", err, ErrTokenConcurrentLimit)
	}

	for i := 0; i < DefaultAPITokenConcurrentLimit; i++ {
		middleware.ReleaseConcurrency(token)
	}

	if err := middleware.AcquireConcurrency(token); err != nil {
		t.Fatalf("AcquireConcurrency() after release error = %v", err)
	}
}

func TestTokenAuthConcurrentLimitByName(t *testing.T) {
	repo := newTokenAuthTestRepo()
	cachedRepo := cached.NewAPITokenRepository(repo)
	middleware := NewTokenAuthMiddleware(cachedRepo, tokenAuthTestSettingRepo{})
	token := &domain.APIToken{ID: 0, Token: "maxx_test_token_no_id", IsEnabled: true}

	for i := 0; i < DefaultAPITokenConcurrentLimit; i++ {
		if err := middleware.AcquireConcurrency(token); err != nil {
			t.Fatalf("AcquireConcurrency() attempt %d error = %v", i+1, err)
		}
	}

	if err := middleware.AcquireConcurrency(token); err != ErrTokenConcurrentLimit {
		t.Fatalf("AcquireConcurrency() over limit error = %v, want %v", err, ErrTokenConcurrentLimit)
	}

	for i := 0; i < DefaultAPITokenConcurrentLimit; i++ {
		middleware.ReleaseConcurrency(token)
	}

	if err := middleware.AcquireConcurrency(token); err != nil {
		t.Fatalf("AcquireConcurrency() after release error = %v", err)
	}
}

func TestTokenAuthAcquireConcurrencyRejectsEmptyNameFallback(t *testing.T) {
	repo := newTokenAuthTestRepo()
	cachedRepo := cached.NewAPITokenRepository(repo)
	middleware := NewTokenAuthMiddleware(cachedRepo, tokenAuthTestSettingRepo{})
	token := &domain.APIToken{ID: 0, Token: "", IsEnabled: true}

	if err := middleware.AcquireConcurrency(token); err != ErrInvalidToken {
		t.Fatalf("AcquireConcurrency() error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestTokenAuthResolveToken(t *testing.T) {
	repo := newTokenAuthTestRepo()
	cachedRepo := cached.NewAPITokenRepository(repo)
	token := &domain.APIToken{
		TenantID:    domain.DefaultTenantID,
		Token:       "maxx_test_token_resolve",
		TokenPrefix: "maxx_test...",
		Name:        "resolve-token",
		IsEnabled:   true,
	}
	if err := cachedRepo.Create(token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	middleware := NewTokenAuthMiddleware(cachedRepo, tokenAuthTestSettingRepo{})

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "http://example.test/v1/messages", nil)
		req.Header.Set("x-api-key", token.Token)

		resolved, err := middleware.ResolveToken(req)
		if err != nil {
			t.Fatalf("ResolveToken() error = %v", err)
		}
		if resolved == nil || resolved.Token != token.Token {
			t.Fatalf("ResolveToken() token = %#v, want token %q", resolved, token.Token)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "http://example.test/v1/messages", nil)
		req.Header.Set("x-api-key", "not_maxx_token")

		resolved, err := middleware.ResolveToken(req)
		if err != ErrInvalidToken {
			t.Fatalf("ResolveToken() error = %v, want %v", err, ErrInvalidToken)
		}
		if resolved != nil {
			t.Fatalf("ResolveToken() token = %#v, want nil", resolved)
		}
	})
}
