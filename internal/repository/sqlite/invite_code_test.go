package sqlite

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func newInviteTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := fmt.Sprintf("file:invitecode_%d?mode=memory&cache=shared&_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)", time.Now().UnixNano())
	db, err := NewDBWithDSN(dsn)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	return db
}

func TestInviteCodeConsume_Expired(t *testing.T) {
	db := newInviteTestDB(t)
	repo := NewInviteCodeRepository(db)

	code := &domain.InviteCode{
		TenantID:   1,
		CodeHash:   "hash-expired",
		CodePrefix: "HASH",
		Status:     domain.InviteCodeStatusActive,
		MaxUses:    1,
		UsedCount:  0,
		ExpiresAt:  ptrTime(time.Now().Add(-time.Hour)),
	}
	if err := repo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}

	if _, err := repo.Consume(1, "hash-expired", time.Now()); err != domain.ErrInviteCodeExpired {
		t.Fatalf("consume error = %v, want %v", err, domain.ErrInviteCodeExpired)
	}
}

func TestInviteCodeConsume_Disabled(t *testing.T) {
	db := newInviteTestDB(t)
	repo := NewInviteCodeRepository(db)

	code := &domain.InviteCode{
		TenantID:   1,
		CodeHash:   "hash-disabled",
		CodePrefix: "HASH",
		Status:     domain.InviteCodeStatusDisabled,
		MaxUses:    1,
		UsedCount:  0,
	}
	if err := repo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}

	if _, err := repo.Consume(1, "hash-disabled", time.Now()); err != domain.ErrInviteCodeDisabled {
		t.Fatalf("consume error = %v, want %v", err, domain.ErrInviteCodeDisabled)
	}
}

func TestInviteCodeConsume_Exhausted(t *testing.T) {
	db := newInviteTestDB(t)
	repo := NewInviteCodeRepository(db)

	code := &domain.InviteCode{
		TenantID:   1,
		CodeHash:   "hash-exhausted",
		CodePrefix: "HASH",
		Status:     domain.InviteCodeStatusActive,
		MaxUses:    1,
		UsedCount:  1,
	}
	if err := repo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}

	if _, err := repo.Consume(1, "hash-exhausted", time.Now()); err != domain.ErrInviteCodeExhausted {
		t.Fatalf("consume error = %v, want %v", err, domain.ErrInviteCodeExhausted)
	}
}

func TestInviteCodeConsume_Concurrent(t *testing.T) {
	db := newInviteTestDB(t)
	repo := NewInviteCodeRepository(db)

	code := &domain.InviteCode{
		TenantID:   1,
		CodeHash:   "hash-concurrent",
		CodePrefix: "HASH",
		Status:     domain.InviteCodeStatusActive,
		MaxUses:    1,
		UsedCount:  0,
	}
	if err := repo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Consume(1, "hash-concurrent", time.Now())
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		failures++
		if !errors.Is(err, domain.ErrInviteCodeExhausted) {
			t.Fatalf("unexpected consume error: %v", err)
		}
	}
	if successes+failures != 10 {
		t.Fatalf("total results = %d, want 10", successes+failures)
	}
	if successes != 1 {
		t.Fatalf("success = %d, want 1", successes)
	}

	updated, err := repo.GetByID(1, code.ID)
	if err != nil {
		t.Fatalf("get code: %v", err)
	}
	if updated.UsedCount != 1 {
		t.Fatalf("usedCount = %d, want 1", updated.UsedCount)
	}
}

func TestInviteCodeUpdate_TenantScopeAndSoftDelete(t *testing.T) {
	db := newInviteTestDB(t)
	repo := NewInviteCodeRepository(db)

	code := &domain.InviteCode{
		TenantID:   1,
		CodeHash:   "hash-update-1",
		CodePrefix: "HASH",
		Status:     domain.InviteCodeStatusActive,
		MaxUses:    1,
		UsedCount:  0,
	}
	if err := repo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}

	code.Note = "wrong-tenant"
	if err := repo.Update(2, code); err != domain.ErrNotFound {
		t.Fatalf("update wrong tenant error = %v, want %v", err, domain.ErrNotFound)
	}

	if err := repo.Delete(1, code.ID); err != nil {
		t.Fatalf("delete code: %v", err)
	}

	code.Note = "deleted"
	if err := repo.Update(1, code); err != domain.ErrNotFound {
		t.Fatalf("update deleted error = %v, want %v", err, domain.ErrNotFound)
	}

	code2 := &domain.InviteCode{
		TenantID:   1,
		CodeHash:   "hash-update-2",
		CodePrefix: "HASH",
		Status:     domain.InviteCodeStatusActive,
		MaxUses:    1,
		UsedCount:  0,
	}
	if err := repo.Create(code2); err != nil {
		t.Fatalf("create code2: %v", err)
	}

	code2.Note = "updated"
	if err := repo.Update(1, code2); err != nil {
		t.Fatalf("update code2: %v", err)
	}

	updated, err := repo.GetByID(1, code2.ID)
	if err != nil {
		t.Fatalf("get updated: %v", err)
	}
	if updated.Note != "updated" {
		t.Fatalf("note = %q, want %q", updated.Note, "updated")
	}
}

func TestInviteCodeUpdate_NoChangeDoesNotReturnNotFound(t *testing.T) {
	db := newInviteTestDB(t)
	repo := NewInviteCodeRepository(db)

	code := &domain.InviteCode{
		TenantID:   1,
		CodeHash:   "hash-nochange-1",
		CodePrefix: "HASH",
		Status:     domain.InviteCodeStatusActive,
		MaxUses:    1,
		UsedCount:  0,
		Note:       "same",
	}
	if err := repo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}

	fixed := time.UnixMilli(1710000000000)
	if err := db.gorm.Model(&InviteCode{}).
		Where("id = ?", code.ID).
		Updates(map[string]any{"updated_at": toTimestamp(fixed)}).Error; err != nil {
		t.Fatalf("set updated_at: %v", err)
	}

	repo.SetNowFunc(func() time.Time { return fixed })
	t.Cleanup(func() { repo.SetNowFunc(nil) })

	if err := repo.Update(1, code); err != nil {
		t.Fatalf("update no-change error = %v, want nil", err)
	}
}

func TestInviteCodeRollbackConsume_Idempotent(t *testing.T) {
	db := newInviteTestDB(t)
	codeRepo := NewInviteCodeRepository(db)
	usageRepo := NewInviteCodeUsageRepository(db)

	code := &domain.InviteCode{
		TenantID:   1,
		CodeHash:   "hash-rollback",
		CodePrefix: "HASH",
		Status:     domain.InviteCodeStatusActive,
		MaxUses:    1,
		UsedCount:  1,
	}
	if err := codeRepo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}

	usage := &domain.InviteCodeUsage{
		TenantID:     1,
		InviteCodeID: code.ID,
		UserID:       0,
		Username:     "test",
		UsedAt:       time.Now(),
		Result:       "failed",
	}
	if err := usageRepo.Create(usage); err != nil {
		t.Fatalf("create usage: %v", err)
	}

	if err := codeRepo.RollbackConsume(1, usage.ID); err != nil {
		t.Fatalf("rollback consume: %v", err)
	}
	if err := codeRepo.RollbackConsume(1, usage.ID); err != nil {
		t.Fatalf("rollback consume again: %v", err)
	}

	updated, err := codeRepo.GetByID(1, code.ID)
	if err != nil {
		t.Fatalf("get code: %v", err)
	}
	if updated.UsedCount != 0 {
		t.Fatalf("usedCount = %d, want 0", updated.UsedCount)
	}

	usages, err := usageRepo.ListByCodeID(1, code.ID)
	if err != nil {
		t.Fatalf("list usages: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("usages = %d, want 1", len(usages))
	}
	if !usages[0].RolledBack {
		t.Fatalf("usage.RolledBack = false, want true")
	}
}

func TestInviteCodeConsumeAndCreateUser_RollbackOnUserCreateFailure(t *testing.T) {
	db := newInviteTestDB(t)
	codeRepo := NewInviteCodeRepository(db)
	userRepo := NewUserRepository(db)

	code := &domain.InviteCode{
		TenantID:   1,
		CodeHash:   "hash-atomic",
		CodePrefix: "HASH",
		Status:     domain.InviteCodeStatusActive,
		MaxUses:    1,
		UsedCount:  0,
	}
	if err := codeRepo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}

	existing := &domain.User{
		TenantID:     1,
		Username:     "dup-user",
		PasswordHash: "hash",
		Role:         domain.UserRoleMember,
		Status:       domain.UserStatusPending,
	}
	if err := userRepo.Create(existing); err != nil {
		t.Fatalf("create user: %v", err)
	}

	user := &domain.User{
		TenantID:     1,
		Username:     "dup-user",
		PasswordHash: "hash",
		Role:         domain.UserRoleMember,
		Status:       domain.UserStatusPending,
	}

	if _, err := codeRepo.ConsumeAndCreateUser(1, "hash-atomic", time.Now(), user); err == nil {
		t.Fatalf("expected error, got nil")
	}

	updated, err := codeRepo.GetByID(1, code.ID)
	if err != nil {
		t.Fatalf("get code: %v", err)
	}
	if updated.UsedCount != 0 {
		t.Fatalf("usedCount = %d, want 0", updated.UsedCount)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
