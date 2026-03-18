package sqlite

import (
	"path/filepath"
	"testing"

	gormsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDBDialectorName(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	if db.Dialector() != "sqlite" {
		t.Fatalf("Expected dialector 'sqlite', got %q", db.Dialector())
	}
}

func TestNewDBWithDSN_CodexQuotaMigrationHandlesDuplicateHistoricalIdentities(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "codex-quota-root-cause.db")
	gormDB, err := gorm.Open(gormsqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get seed sql.DB: %v", err)
	}
	defer sqlDB.Close()

	seedSQL := []string{
		`CREATE TABLE codex_quotas (
			id TEXT PRIMARY KEY,
			created_at INTEGER DEFAULT 0,
			updated_at INTEGER DEFAULT 0,
			deleted_at INTEGER DEFAULT 0,
			tenant_id INTEGER NOT NULL,
			identity_key TEXT,
			email TEXT,
			account_id TEXT,
			plan_type TEXT,
			is_forbidden INTEGER DEFAULT 0,
			primary_window TEXT,
			secondary_window TEXT,
			code_review_window TEXT
		)`,
		`CREATE UNIQUE INDEX idx_codex_quotas_tenant_email ON codex_quotas(tenant_id, email)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-1', 1, NULL, 'first@example.com', 'acct-1', 100)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-2', 1, NULL, 'second@example.com', 'acct-1', 200)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-3', 1, NULL, 'third@example.com', 'acct-2', 150)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-4', 2, NULL, 'other-tenant@example.com', 'acct-1', 120)`,
	}
	for _, sql := range seedSQL {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("seed fixture: %v", err)
		}
	}

	db, err := NewDBWithDSN("sqlite://" + dsn)
	if err != nil {
		t.Fatalf("NewDBWithDSN should survive duplicate historical identities: %v", err)
	}
	defer db.Close()

	var count int64
	if err := db.GormDB().Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 1 AND identity_key = 'account:acct-1'`).Scan(&count).Error; err != nil {
		t.Fatalf("count migrated identities: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected duplicate historical identity rows to collapse to 1, got %d", count)
	}
}

func TestNewDBWithDSN_CodexQuotaMigrationPreservesDeletedHistory(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "codex-quota-deleted-history.db")
	gormDB, err := gorm.Open(gormsqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get seed sql.DB: %v", err)
	}
	defer sqlDB.Close()

	seedSQL := []string{
		`CREATE TABLE codex_quotas (
			id TEXT PRIMARY KEY,
			created_at INTEGER DEFAULT 0,
			updated_at INTEGER DEFAULT 0,
			deleted_at INTEGER DEFAULT 0,
			tenant_id INTEGER NOT NULL,
			identity_key TEXT,
			email TEXT,
			account_id TEXT,
			plan_type TEXT,
			is_forbidden INTEGER DEFAULT 0,
			primary_window TEXT,
			secondary_window TEXT,
			code_review_window TEXT
		)`,
		`CREATE UNIQUE INDEX idx_codex_quotas_tenant_email ON codex_quotas(tenant_id, email)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, deleted_at, updated_at) VALUES ('row-deleted', 1, NULL, 'old@example.com', 'acct-1', 111, 100)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, deleted_at, updated_at) VALUES ('row-active', 1, NULL, 'current@example.com', 'acct-1', 0, 200)`,
	}
	for _, sql := range seedSQL {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("seed fixture: %v", err)
		}
	}

	db, err := NewDBWithDSN("sqlite://" + dsn)
	if err != nil {
		t.Fatalf("NewDBWithDSN should preserve deleted history while migrating: %v", err)
	}
	defer db.Close()

	var activeCount int64
	if err := db.GormDB().Raw(`
		SELECT COUNT(*)
		FROM codex_quotas
		WHERE tenant_id = 1 AND identity_key = 'account:acct-1' AND deleted_at = 0
	`).Scan(&activeCount).Error; err != nil {
		t.Fatalf("count active rows: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("expected one active row after migration, got %d", activeCount)
	}

	var deletedCount int64
	if err := db.GormDB().Raw(`
		SELECT COUNT(*)
		FROM codex_quotas
		WHERE tenant_id = 1 AND identity_key = 'account:acct-1' AND deleted_at != 0
	`).Scan(&deletedCount).Error; err != nil {
		t.Fatalf("count deleted rows: %v", err)
	}
	if deletedCount != 1 {
		t.Fatalf("expected deleted row to be preserved after migration, got %d", deletedCount)
	}
}
