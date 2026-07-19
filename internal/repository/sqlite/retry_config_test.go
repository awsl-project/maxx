package sqlite

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestForceRetryUpstreamErrorsMigrationCopiesLegacySetting(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewRetryConfigRepository(db)
	config := &domain.RetryConfig{
		TenantID:        1,
		Name:            "Default Config",
		IsDefault:       true,
		MaxRetries:      1,
		InitialInterval: time.Second,
		BackoffRate:     1,
		MaxInterval:     time.Second,
	}
	if err := repo.Create(config); err != nil {
		t.Fatalf("create retry config: %v", err)
	}
	if err := db.gorm.Create(&SystemSetting{Key: "force_retry_upstream_errors", Value: "true"}).Error; err != nil {
		t.Fatalf("create legacy setting: %v", err)
	}

	if err := runForceRetryUpstreamErrorsRetryConfigMigration(db.gorm); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	got, err := repo.GetByID(1, config.ID)
	if err != nil {
		t.Fatalf("get retry config: %v", err)
	}
	if !got.ForceRetryUpstreamErrors {
		t.Fatal("ForceRetryUpstreamErrors = false, want true copied from legacy setting")
	}
}

func TestRetryConfigForceRetryUpstreamErrorsDefaultsFalse(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	repo := NewRetryConfigRepository(db)
	config := &domain.RetryConfig{
		TenantID:        1,
		Name:            "Default Config",
		IsDefault:       true,
		MaxRetries:      1,
		InitialInterval: time.Second,
		BackoffRate:     1,
		MaxInterval:     time.Second,
	}
	if err := repo.Create(config); err != nil {
		t.Fatalf("create retry config: %v", err)
	}

	got, err := repo.GetByID(1, config.ID)
	if err != nil {
		t.Fatalf("get retry config: %v", err)
	}
	if got.ForceRetryUpstreamErrors {
		t.Fatal("ForceRetryUpstreamErrors = true, want default false")
	}
}
