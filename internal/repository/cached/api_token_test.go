package cached

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type apiTokenTestRepo struct {
	token *domain.APIToken
}

func (r *apiTokenTestRepo) Create(token *domain.APIToken) error {
	if token.ID == 0 {
		token.ID = 1
	}
	clone := *token
	r.token = &clone
	return nil
}

func (r *apiTokenTestRepo) Update(token *domain.APIToken) error {
	clone := *token
	r.token = &clone
	return nil
}

func (r *apiTokenTestRepo) Delete(tenantID uint64, id uint64) error { return nil }

func (r *apiTokenTestRepo) DeleteExpired(tenantID uint64, now time.Time, inactiveExpiry time.Duration) ([]*domain.APIToken, error) {
	if r.token == nil {
		return []*domain.APIToken{}, nil
	}
	clone := *r.token
	r.token = nil
	return []*domain.APIToken{&clone}, nil
}

func (r *apiTokenTestRepo) GetByID(tenantID uint64, id uint64) (*domain.APIToken, error) {
	if r.token == nil || r.token.ID != id {
		return nil, domain.ErrNotFound
	}
	clone := *r.token
	return &clone, nil
}

func (r *apiTokenTestRepo) GetByToken(tenantID uint64, token string) (*domain.APIToken, error) {
	if r.token == nil || r.token.Token != token {
		return nil, domain.ErrNotFound
	}
	clone := *r.token
	return &clone, nil
}

func (r *apiTokenTestRepo) List(tenantID uint64) ([]*domain.APIToken, error) {
	if r.token == nil {
		return nil, nil
	}
	clone := *r.token
	return []*domain.APIToken{&clone}, nil
}

func (r *apiTokenTestRepo) UpdateLastSeen(tenantID uint64, id uint64, lastIP string, lastSeenAt time.Time) error {
	if r.token == nil || r.token.ID != id {
		return domain.ErrNotFound
	}
	r.token.UseCount++
	r.token.LastUsedAt = &lastSeenAt
	if lastIP != "" {
		r.token.LastIP = lastIP
		r.token.LastIPAt = &lastSeenAt
	}
	return nil
}

func TestAPITokenRepositoryUpdateLastSeenKeepsLastIPWhenIPIsEmpty(t *testing.T) {
	baseRepo := &apiTokenTestRepo{}
	repo := NewAPITokenRepository(baseRepo)
	token := &domain.APIToken{
		TenantID:    1,
		Token:       "maxx_test_token_123",
		TokenPrefix: "maxx_test...",
		Name:        "test-token",
		IsEnabled:   true,
	}
	if err := repo.Create(token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	firstSeenAt := time.Unix(1710000000, 0).UTC()
	if err := repo.UpdateLastSeen(token.TenantID, token.ID, "198.51.100.10", firstSeenAt); err != nil {
		t.Fatalf("UpdateLastSeen(first) error = %v", err)
	}

	secondSeenAt := firstSeenAt.Add(2 * time.Minute)
	if err := repo.UpdateLastSeen(token.TenantID, token.ID, "", secondSeenAt); err != nil {
		t.Fatalf("UpdateLastSeen(second) error = %v", err)
	}

	cachedToken, err := repo.GetByID(token.TenantID, token.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if cachedToken.UseCount != 2 {
		t.Fatalf("UseCount = %d, want 2", cachedToken.UseCount)
	}
	if cachedToken.LastUsedAt == nil || !cachedToken.LastUsedAt.Equal(secondSeenAt) {
		t.Fatalf("LastUsedAt = %v, want %v", cachedToken.LastUsedAt, secondSeenAt)
	}
	if cachedToken.LastIP != "198.51.100.10" {
		t.Fatalf("LastIP = %q, want %q", cachedToken.LastIP, "198.51.100.10")
	}
	if cachedToken.LastIPAt == nil || !cachedToken.LastIPAt.Equal(firstSeenAt) {
		t.Fatalf("LastIPAt = %v, want %v", cachedToken.LastIPAt, firstSeenAt)
	}
}

func TestAPITokenRepositoryDeleteExpiredClearsTokenCache(t *testing.T) {
	baseRepo := &apiTokenTestRepo{}
	repo := NewAPITokenRepository(baseRepo)
	token := &domain.APIToken{
		TenantID:    1,
		Token:       "maxx_expired_cached_token",
		TokenPrefix: "maxx_exp...",
		Name:        "expired-cached-token",
		IsEnabled:   true,
	}
	if err := repo.Create(token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := repo.GetByToken(1, token.Token); err != nil {
		t.Fatalf("GetByToken(before cleanup) error = %v", err)
	}

	deleted, err := repo.DeleteExpired(1, time.Now(), domain.APITokenInactiveExpiry)
	if err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("deleted count = %d, want 1", len(deleted))
	}
	if _, err := repo.GetByToken(1, token.Token); err != domain.ErrNotFound {
		t.Fatalf("GetByToken(after cleanup) error = %v, want ErrNotFound", err)
	}
}
