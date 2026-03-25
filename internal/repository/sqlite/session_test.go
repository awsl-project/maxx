package sqlite

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestSessionListOrdersByUpdatedAtDesc(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewSessionRepository(db)
	first := &domain.Session{
		TenantID:   1,
		SessionID:  "session-first",
		ClientType: domain.ClientTypeCodex,
	}
	second := &domain.Session{
		TenantID:   1,
		SessionID:  "session-second",
		ClientType: domain.ClientTypeCodex,
	}

	if err := repo.Create(first); err != nil {
		t.Fatalf("Create first session: %v", err)
	}
	if err := repo.Create(second); err != nil {
		t.Fatalf("Create second session: %v", err)
	}

	if err := repo.Touch(1, first.SessionID, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("Touch first session: %v", err)
	}

	sessions, err := repo.List(1)
	if err != nil {
		t.Fatalf("List sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("Expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].SessionID != first.SessionID {
		t.Fatalf("Expected touched session %q first, got %q", first.SessionID, sessions[0].SessionID)
	}
}

func TestSessionDeleteOlderThanHardDeletesExpiredRows(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewSessionRepository(db)
	oldSession := &domain.Session{
		TenantID:   1,
		SessionID:  "session-old",
		ClientType: domain.ClientTypeClaude,
	}
	recentSession := &domain.Session{
		TenantID:   1,
		SessionID:  "session-recent",
		ClientType: domain.ClientTypeClaude,
	}

	if err := repo.Create(oldSession); err != nil {
		t.Fatalf("Create old session: %v", err)
	}
	if err := repo.Create(recentSession); err != nil {
		t.Fatalf("Create recent session: %v", err)
	}

	now := time.Now()
	if err := repo.Touch(1, oldSession.SessionID, now.Add(-72*time.Hour)); err != nil {
		t.Fatalf("Touch old session: %v", err)
	}
	if err := repo.Touch(1, recentSession.SessionID, now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Touch recent session: %v", err)
	}

	deleted, err := repo.DeleteOlderThan(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("Expected 1 deleted session, got %d", deleted)
	}

	sessions, err := repo.List(1)
	if err != nil {
		t.Fatalf("List sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("Expected 1 remaining session, got %d", len(sessions))
	}
	if sessions[0].SessionID != recentSession.SessionID {
		t.Fatalf("Expected remaining session %q, got %q", recentSession.SessionID, sessions[0].SessionID)
	}

	var rawCount int64
	if err := db.GormDB().Model(&Session{}).Where("session_id = ?", oldSession.SessionID).Count(&rawCount).Error; err != nil {
		t.Fatalf("Count raw sessions: %v", err)
	}
	if rawCount != 0 {
		t.Fatalf("Expected old session to be hard-deleted, raw count=%d", rawCount)
	}
}

func TestSessionDeleteOlderThanHardDeletesExpiredSoftDeletedRows(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewSessionRepository(db)
	session := &domain.Session{
		TenantID:   1,
		SessionID:  "session-soft-deleted-old",
		ClientType: domain.ClientTypeCodex,
	}

	if err := repo.Create(session); err != nil {
		t.Fatalf("Create session: %v", err)
	}
	if err := repo.Delete(session.ID); err != nil {
		t.Fatalf("Delete session: %v", err)
	}

	expiredAt := time.Now().Add(-72 * time.Hour)
	if err := db.GormDB().
		Model(&Session{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{
			"updated_at": toTimestamp(expiredAt),
			"deleted_at": toTimestamp(expiredAt),
		}).Error; err != nil {
		t.Fatalf("Age soft-deleted session: %v", err)
	}

	deleted, err := repo.DeleteOlderThan(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("Expected 1 deleted session, got %d", deleted)
	}

	var rawCount int64
	if err := db.GormDB().Model(&Session{}).Where("id = ?", session.ID).Count(&rawCount).Error; err != nil {
		t.Fatalf("Count raw sessions: %v", err)
	}
	if rawCount != 0 {
		t.Fatalf("Expected soft-deleted expired session to be hard-deleted, raw count=%d", rawCount)
	}
}

func TestSessionTouchReturnsNotFoundWhenSessionMissing(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewSessionRepository(db)
	err = repo.Touch(1, "missing-session", time.Now())
	if err == nil {
		t.Fatal("expected Touch to return ErrNotFound for missing session")
	}
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
