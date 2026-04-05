package core

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type fakeSessionRepo struct {
	deleteCalls int
	lastBefore  time.Time
}

func (f *fakeSessionRepo) Create(session *domain.Session) error {
	return nil
}

func (f *fakeSessionRepo) Update(session *domain.Session) error {
	return nil
}

func (f *fakeSessionRepo) Touch(tenantID uint64, sessionID string, touchedAt time.Time) error {
	return nil
}

func (f *fakeSessionRepo) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeSessionRepo) List(tenantID uint64) ([]*domain.Session, error) {
	return nil, nil
}

func (f *fakeSessionRepo) DeleteOlderThan(before time.Time) (int64, error) {
	f.deleteCalls++
	f.lastBefore = before
	return 0, nil
}

type fakeSettingRepo struct {
	values map[string]string
}

func (f *fakeSettingRepo) Get(key string) (string, error) {
	if f.values == nil {
		return "", nil
	}
	return f.values[key], nil
}

func (f *fakeSettingRepo) Set(key, value string) error {
	if f.values == nil {
		f.values = make(map[string]string)
	}
	f.values[key] = value
	return nil
}

func (f *fakeSettingRepo) GetAll() ([]*domain.SystemSetting, error) {
	return nil, nil
}

func (f *fakeSettingRepo) Delete(key string) error {
	if f.values != nil {
		delete(f.values, key)
	}
	return nil
}

func TestCleanupOldSessionsUsesDefaultRetention(t *testing.T) {
	sessionRepo := &fakeSessionRepo{}
	deps := BackgroundTaskDeps{
		SessionRepo: sessionRepo,
		Settings:    &fakeSettingRepo{},
	}

	start := time.Now()
	deps.cleanupOldSessions()
	end := time.Now()

	if sessionRepo.deleteCalls != 1 {
		t.Fatalf("Expected cleanup to run once, got %d", sessionRepo.deleteCalls)
	}

	expectedMin := start.Add(-defaultSessionRetentionHours * time.Hour).Add(-2 * time.Second)
	expectedMax := end.Add(-defaultSessionRetentionHours * time.Hour).Add(2 * time.Second)
	if sessionRepo.lastBefore.Before(expectedMin) || sessionRepo.lastBefore.After(expectedMax) {
		t.Fatalf("Expected cleanup cutoff near %v..%v, got %v", expectedMin, expectedMax, sessionRepo.lastBefore)
	}
}

func TestCleanupOldSessionsRespectsDisabledSetting(t *testing.T) {
	sessionRepo := &fakeSessionRepo{}
	deps := BackgroundTaskDeps{
		SessionRepo: sessionRepo,
		Settings: &fakeSettingRepo{
			values: map[string]string{
				domain.SettingKeySessionRetentionHours: "0",
			},
		},
	}

	deps.cleanupOldSessions()

	if sessionRepo.deleteCalls != 0 {
		t.Fatalf("Expected cleanup to be disabled, got %d calls", sessionRepo.deleteCalls)
	}
}
