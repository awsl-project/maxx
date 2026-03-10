package sqlite

import (
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
	success := 0
	mu := sync.Mutex{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.Consume(1, "hash-concurrent", time.Now()); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if success != 1 {
		t.Fatalf("success = %d, want 1", success)
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

	prevNow := nowFunc
	nowFunc = func() time.Time { return fixed }
	t.Cleanup(func() { nowFunc = prevNow })

	if err := repo.Update(1, code); err != nil {
		t.Fatalf("update no-change error = %v, want nil", err)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
