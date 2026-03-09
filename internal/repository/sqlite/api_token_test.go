package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestAPITokenRepository_CreateStoresTokenDigest(t *testing.T) {
	db, err := NewDB(filepath.Join(t.TempDir(), "token.db"))
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewAPITokenRepository(db)
	plain := domain.APITokenPrefix + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	token := &domain.APIToken{
		TenantID:    domain.DefaultTenantID,
		Token:       plain,
		TokenPrefix: plain[:domain.APITokenPrefixDisplayLen] + "...",
		Name:        "hashed-token",
		IsEnabled:   true,
	}

	if err := repo.Create(token); err != nil {
		t.Fatalf("create token: %v", err)
	}

	hashed := domain.HashAPIToken(plain)
	if token.Token != hashed {
		t.Fatalf("token stored in domain = %q, want hash %q", token.Token, hashed)
	}

	var stored APIToken
	if err := db.gorm.First(&stored, token.ID).Error; err != nil {
		t.Fatalf("load stored token: %v", err)
	}
	if stored.Token != hashed {
		t.Fatalf("db token = %q, want hash %q", stored.Token, hashed)
	}

	loaded, err := repo.GetByToken(domain.DefaultTenantID, plain)
	if err != nil {
		t.Fatalf("get by plain token: %v", err)
	}
	if loaded.ID != token.ID {
		t.Fatalf("loaded id = %d, want %d", loaded.ID, token.ID)
	}
	if loaded.Token != hashed {
		t.Fatalf("loaded token = %q, want hash %q", loaded.Token, hashed)
	}
}
