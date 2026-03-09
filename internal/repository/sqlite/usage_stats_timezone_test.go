package sqlite

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestResolveTimezoneUsesConfiguredSetting(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	repo := NewUsageStatsRepository(db)
	settingRepo := NewSystemSettingRepository(db)
	if err := settingRepo.Set(domain.SettingKeyTimezone, "UTC"); err != nil {
		t.Fatalf("failed to set timezone: %v", err)
	}

	tz := repo.resolveTimezone()
	if tz.identifier != "UTC" {
		t.Fatalf("identifier = %q, want UTC", tz.identifier)
	}
	if tz.location.String() != "UTC" {
		t.Fatalf("location = %q, want UTC", tz.location.String())
	}
}

func TestResolveTimezoneFallsBackToDeploymentTimezoneWhenUnset(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	originalLocal := time.Local
	time.Local = time.FixedZone("Deployment", 9*60*60)
	t.Cleanup(func() {
		time.Local = originalLocal
	})

	repo := NewUsageStatsRepository(db)
	tz := repo.resolveTimezone()
	if tz.identifier != "UTC+09:00" {
		t.Fatalf("identifier = %q, want UTC+09:00", tz.identifier)
	}
	if tz.location == nil {
		t.Fatal("location should not be nil")
	}
}

func TestResolveTimezoneFallsBackToDeploymentTimezoneWhenConfiguredTimezoneInvalid(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	originalLocal := time.Local
	time.Local = time.FixedZone("Deployment", -5*60*60)
	t.Cleanup(func() {
		time.Local = originalLocal
	})

	repo := NewUsageStatsRepository(db)
	settingRepo := NewSystemSettingRepository(db)
	if err := settingRepo.Set(domain.SettingKeyTimezone, "Mars/Base"); err != nil {
		t.Fatalf("failed to set invalid timezone: %v", err)
	}

	tz := repo.resolveTimezone()
	if tz.identifier != "UTC-05:00" {
		t.Fatalf("identifier = %q, want UTC-05:00", tz.identifier)
	}
	if tz.location == nil {
		t.Fatal("location should not be nil")
	}
}
