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

func TestNewDBWithDSN_CodexQuotaMigrationHandlesPreexistingIdentityIndex(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "codex-quota-preexisting-identity-index.db")
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
		`CREATE UNIQUE INDEX idx_codex_quotas_tenant_identity ON codex_quotas(tenant_id, identity_key)`,
		`CREATE UNIQUE INDEX idx_codex_quotas_email ON codex_quotas(email)`,
		`CREATE UNIQUE INDEX idx_codex_quotas_tenant_email ON codex_quotas(tenant_id, email)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-8', 1, NULL, 'cnc6n2io9xvfev2mtm5t6hu8@example.com', 'e94ce011-80f4-490b-b285-d3109db72b0e', 1773456445476)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-11', 1, NULL, 'fcew8ua8r6u6zekrwqfi60nl@example.com', 'e94ce011-80f4-490b-b285-d3109db72b0e', 1773456444987)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-7', 1, NULL, 'puckxqnzu6ktt7k4bcevlw06@example.com', 'e94ce011-80f4-490b-b285-d3109db72b0e', 1773456443639)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-9', 1, NULL, 'n7e87dj5hxv2c2m4u0e6l5zo@example.com', 'e94ce011-80f4-490b-b285-d3109db72b0e', 1773456443366)`,
	}
	for _, sql := range seedSQL {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("seed fixture: %v", err)
		}
	}

	db, err := NewDBWithDSN("sqlite://" + dsn)
	if err != nil {
		t.Fatalf("NewDBWithDSN should survive preexisting identity index: %v", err)
	}
	defer db.Close()

	var count int64
	if err := db.GormDB().Raw(`
		SELECT COUNT(*)
		FROM codex_quotas
		WHERE tenant_id = 1 AND identity_key = 'account:e94ce011-80f4-490b-b285-d3109db72b0e' AND deleted_at = 0
	`).Scan(&count).Error; err != nil {
		t.Fatalf("count migrated rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected rows to collapse to one active account identity, got %d", count)
	}

	var rows []struct {
		Name   string `gorm:"column:name"`
		Unique int    `gorm:"column:unique"`
	}
	if err := db.GormDB().Raw(`PRAGMA index_list('codex_quotas')`).Scan(&rows).Error; err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	for _, row := range rows {
		if row.Name == "idx_codex_quotas_email" && row.Unique != 0 {
			t.Fatalf("expected idx_codex_quotas_email to be recreated as non-unique, got unique=%d", row.Unique)
		}
	}
}
