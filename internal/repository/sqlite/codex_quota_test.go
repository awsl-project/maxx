package sqlite

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestCodexQuotaRepository_UpsertUsesIdentityKey(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewCodexQuotaRepository(db)

	quota1 := &domain.CodexQuota{
		TenantID:  1,
		Email:     "same@example.com",
		AccountID: "acct-1",
		PlanType:  "team",
	}
	if err := repo.Upsert(quota1); err != nil {
		t.Fatalf("Upsert quota1 failed: %v", err)
	}

	quota2 := &domain.CodexQuota{
		TenantID:  1,
		Email:     "same@example.com",
		AccountID: "acct-2",
		PlanType:  "team",
	}
	if err := repo.Upsert(quota2); err != nil {
		t.Fatalf("Upsert quota2 failed: %v", err)
	}

	quota1.PlanType = "team-updated"
	if err := repo.Upsert(quota1); err != nil {
		t.Fatalf("Upsert quota1 update failed: %v", err)
	}

	list, err := repo.List(1)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 quota rows, got %d", len(list))
	}

	got1, err := repo.GetByIdentityKey(1, domain.CodexQuotaIdentityKey(quota1.Email, quota1.AccountID))
	if err != nil {
		t.Fatalf("GetByIdentityKey quota1 failed: %v", err)
	}
	if got1 == nil || got1.AccountID != "acct-1" || got1.PlanType != "team-updated" {
		t.Fatalf("unexpected quota1 row: %#v", got1)
	}

	got2, err := repo.GetByIdentityKey(1, domain.CodexQuotaIdentityKey(quota2.Email, quota2.AccountID))
	if err != nil {
		t.Fatalf("GetByIdentityKey quota2 failed: %v", err)
	}
	if got2 == nil || got2.AccountID != "acct-2" {
		t.Fatalf("unexpected quota2 row: %#v", got2)
	}
}
