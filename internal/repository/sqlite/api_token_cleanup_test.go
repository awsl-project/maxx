package sqlite

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestAPITokenRepositoryDeleteExpiredUsesExplicitAndInactiveExpiry(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	repo := NewAPITokenRepository(db)

	now := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Hour)
	futureExpiry := now.Add(time.Hour)
	inactiveAt := now.Add(-domain.APITokenInactiveExpiry - time.Minute)
	stillActiveAt := now.Add(-domain.APITokenInactiveExpiry + time.Minute)

	seed := []*domain.APIToken{
		{TenantID: 1, Token: "maxx_explicit_expired", TokenPrefix: "maxx_exp", Name: "explicit-expired", IsEnabled: true, ExpiresAt: &expiredAt},
		{TenantID: 1, Token: "maxx_inactive_expired", TokenPrefix: "maxx_ina", Name: "inactive-expired", IsEnabled: true, LastUsedAt: &inactiveAt},
		{TenantID: 1, Token: "maxx_future", TokenPrefix: "maxx_fut", Name: "future", IsEnabled: true, ExpiresAt: &futureExpiry},
		{TenantID: 1, Token: "maxx_recent", TokenPrefix: "maxx_rec", Name: "recent", IsEnabled: true, LastUsedAt: &stillActiveAt},
		{TenantID: 1, Token: "maxx_never_used", TokenPrefix: "maxx_nev", Name: "never-used", IsEnabled: true},
		{TenantID: 2, Token: "maxx_other_tenant_expired", TokenPrefix: "maxx_oth", Name: "other-tenant-expired", IsEnabled: true, ExpiresAt: &expiredAt},
	}
	for _, token := range seed {
		if err := repo.Create(token); err != nil {
			t.Fatalf("Create(%s) error = %v", token.Name, err)
		}
	}

	deleted, err := repo.DeleteExpired(1, now, domain.APITokenInactiveExpiry)
	if err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("deleted count = %d, want 2", len(deleted))
	}
	deletedNames := map[string]bool{}
	for _, token := range deleted {
		deletedNames[token.Name] = true
	}
	for _, name := range []string{"explicit-expired", "inactive-expired"} {
		if !deletedNames[name] {
			t.Fatalf("deleted tokens missing %q: %#v", name, deletedNames)
		}
	}

	remaining, err := repo.List(1)
	if err != nil {
		t.Fatalf("List(tenant 1) error = %v", err)
	}
	remainingNames := map[string]bool{}
	for _, token := range remaining {
		remainingNames[token.Name] = true
	}
	for _, name := range []string{"future", "recent", "never-used"} {
		if !remainingNames[name] {
			t.Fatalf("remaining tokens missing %q: %#v", name, remainingNames)
		}
	}
	for _, name := range []string{"explicit-expired", "inactive-expired"} {
		if remainingNames[name] {
			t.Fatalf("expired token %q still listed after cleanup", name)
		}
	}

	otherTenant, err := repo.List(2)
	if err != nil {
		t.Fatalf("List(tenant 2) error = %v", err)
	}
	if len(otherTenant) != 1 || otherTenant[0].Name != "other-tenant-expired" {
		t.Fatalf("other tenant tokens = %#v, want untouched expired token", otherTenant)
	}
}
