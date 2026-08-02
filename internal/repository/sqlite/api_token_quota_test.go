package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestAPITokenQuotaBalanceDefaultsAndDeductsToZero(t *testing.T) {
	db, err := NewDB(filepath.Join(t.TempDir(), "quota.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	repo := NewAPITokenRepository(db)
	token := &domain.APIToken{
		TenantID:    domain.DefaultTenantID,
		Token:       "maxx_test_quota",
		TokenPrefix: "maxx_test",
		Name:        "quota token",
		IsEnabled:   true,
	}
	if err := repo.Create(token); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(domain.DefaultTenantID, token.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.QuotaBalance != 0 {
		t.Fatalf("default quota balance = %d, want 0", got.QuotaBalance)
	}

	updated, err := repo.AddQuotaBalance(domain.DefaultTenantID, []uint64{token.ID}, 100)
	if err != nil {
		t.Fatalf("AddQuotaBalance: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	got, err = repo.GetByID(domain.DefaultTenantID, token.ID)
	if err != nil {
		t.Fatalf("GetByID after add: %v", err)
	}
	if got.QuotaBalance != 100 {
		t.Fatalf("quota after add = %d, want 100", got.QuotaBalance)
	}

	if err := repo.DeductQuotaBalanceToZero(domain.DefaultTenantID, token.ID, 40); err != nil {
		t.Fatalf("DeductQuotaBalanceToZero partial: %v", err)
	}
	got, _ = repo.GetByID(domain.DefaultTenantID, token.ID)
	if got.QuotaBalance != 60 {
		t.Fatalf("quota after partial deduct = %d, want 60", got.QuotaBalance)
	}

	if err := repo.DeductQuotaBalanceToZero(domain.DefaultTenantID, token.ID, 1000); err != nil {
		t.Fatalf("DeductQuotaBalanceToZero overdraw: %v", err)
	}
	got, _ = repo.GetByID(domain.DefaultTenantID, token.ID)
	if got.QuotaBalance != 0 {
		t.Fatalf("quota after overdraw = %d, want 0", got.QuotaBalance)
	}
}
