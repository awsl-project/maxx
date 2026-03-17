package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

type tokenAuthTestSettingRepo struct{}

func (tokenAuthTestSettingRepo) Get(key string) (string, error) {
	if key == SettingKeyProxyTokenAuthEnabled {
		return "true", nil
	}
	return "", nil
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
