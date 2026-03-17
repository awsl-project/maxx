package sqlite

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func TestIsMySQLDuplicateIndexError(t *testing.T) {
	if !isMySQLDuplicateIndexError(&mysqlDriver.MySQLError{Number: 1061, Message: "Duplicate key name"}) {
		t.Fatalf("expected true for ER_DUP_KEYNAME(1061)")
	}
	if isMySQLDuplicateIndexError(&mysqlDriver.MySQLError{Number: 1146, Message: "Table doesn't exist"}) {
		t.Fatalf("expected false for non-duplicate mysql error")
	}
	if !isMySQLDuplicateIndexError(errors.New("Error 1061: Duplicate key name 'idx_proxy_requests_provider_id'")) {
		t.Fatalf("expected true for duplicate key name string match fallback")
	}
	if isMySQLDuplicateIndexError(errors.New("some other error")) {
		t.Fatalf("expected false for unrelated error")
	}
}

func TestIsMySQLMissingIndexError(t *testing.T) {
	if !isMySQLMissingIndexError(&mysqlDriver.MySQLError{Number: 1091, Message: "Can't DROP"}) {
		t.Fatalf("expected true for ER_CANT_DROP_FIELD_OR_KEY(1091)")
	}
	if !isMySQLMissingIndexError(errors.New("Error 1091: Can't DROP 'idx_x'; check that column/key exists")) {
		t.Fatalf("expected true for missing index string match fallback")
	}
	if isMySQLMissingIndexError(errors.New("some other error")) {
		t.Fatalf("expected false for unrelated error")
	}
}

func TestDedupeCodexQuotaIdentityRows(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	gormDB := db.GormDB()
	prepareCodexQuotaDedupeFixture(t, gormDB)

	if err := dedupeCodexQuotaIdentityRows(gormDB); err != nil {
		t.Fatalf("dedupe identities: %v", err)
	}

	assertCodexQuotaFixtureCounts(t, gormDB)
}

func TestCodexQuotaIdentityMigrationV9Up(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	gormDB := db.GormDB()
	prepareCodexQuotaMigrationFixture(t, gormDB)

	migration := findMigrationByVersion(t, 9)
	if err := migration.Up(gormDB); err != nil {
		t.Fatalf("run migration v9 up: %v", err)
	}

	assertCodexQuotaPostMigrationState(t, gormDB)
	assertIndexExists(t, gormDB, "idx_codex_quotas_tenant_identity", true)
	assertIndexExists(t, gormDB, "idx_codex_quotas_email", false)
	assertIndexMissing(t, gormDB, "idx_codex_quotas_tenant_email")
}

func TestCodexQuotaIdentityMigrationV9DownReturnsIrreversibleError(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	gormDB := db.GormDB()
	prepareCodexQuotaMigrationFixture(t, gormDB)

	migration := findMigrationByVersion(t, 9)
	err = migration.Down(gormDB)
	if err == nil {
		t.Fatal("expected irreversible down migration error")
	}
	if !strings.Contains(err.Error(), "idx_codex_quotas_tenant_email") {
		t.Fatalf("expected error to mention idx_codex_quotas_tenant_email, got %q", err)
	}
	if !strings.Contains(err.Error(), "identity/email") {
		t.Fatalf("expected error to mention CodexQuota identity/email situation, got %q", err)
	}
}

func prepareCodexQuotaDedupeFixture(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	if err := gormDB.Exec(`DROP TABLE IF EXISTS codex_quotas`).Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := gormDB.Exec(`
		CREATE TABLE codex_quotas (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tenant_id INTEGER NOT NULL,
			identity_key TEXT,
			email TEXT,
			account_id TEXT,
			deleted_at INTEGER DEFAULT 0,
			updated_at INTEGER DEFAULT 0
		)
	`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	inserts := []string{
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (1, 'account:acct-1', 'first@example.com', 'acct-1', 100)`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (1, 'account:acct-1', 'second@example.com', 'acct-1', 200)`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (1, 'account:acct-2', 'third@example.com', 'acct-2', 150)`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (2, 'account:acct-1', 'other-tenant@example.com', 'acct-1', 120)`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (1, NULL, 'legacy@example.com', '', 90)`,
	}
	for _, sql := range inserts {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}
}

func prepareCodexQuotaMigrationFixture(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	if err := gormDB.Exec(`DROP TABLE IF EXISTS codex_quotas`).Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := gormDB.Exec(`
		CREATE TABLE codex_quotas (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tenant_id INTEGER NOT NULL,
			identity_key TEXT,
			email TEXT,
			account_id TEXT,
			deleted_at INTEGER DEFAULT 0,
			updated_at INTEGER DEFAULT 0
		)
	`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := gormDB.Exec(`CREATE UNIQUE INDEX idx_codex_quotas_tenant_email ON codex_quotas(tenant_id, email)`).Error; err != nil {
		t.Fatalf("create old unique index: %v", err)
	}
	inserts := []string{
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (1, NULL, 'first@example.com', 'acct-1', 100)`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (1, NULL, 'second@example.com', 'acct-1', 200)`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (1, NULL, 'third@example.com', 'acct-2', 150)`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (2, NULL, 'other-tenant@example.com', 'acct-1', 120)`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id, updated_at) VALUES (1, NULL, 'legacy@example.com', '', 90)`,
	}
	for _, sql := range inserts {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}
}

func assertCodexQuotaFixtureCounts(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	var duplicateCount int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 1 AND identity_key = 'account:acct-1'`).Scan(&duplicateCount).Error; err != nil {
		t.Fatalf("count duplicate rows: %v", err)
	}
	if duplicateCount != 1 {
		t.Fatalf("expected duplicate identity rows to collapse to 1, got %d", duplicateCount)
	}

	var tenant2Count int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 2 AND identity_key = 'account:acct-1'`).Scan(&tenant2Count).Error; err != nil {
		t.Fatalf("count tenant 2 rows: %v", err)
	}
	if tenant2Count != 1 {
		t.Fatalf("expected tenant 2 row to be preserved, got %d", tenant2Count)
	}

	var nullIdentityCount int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 1 AND identity_key IS NULL`).Scan(&nullIdentityCount).Error; err != nil {
		t.Fatalf("count null identity rows: %v", err)
	}
	if nullIdentityCount != 1 {
		t.Fatalf("expected null identity rows to be preserved, got %d", nullIdentityCount)
	}
}

func assertCodexQuotaPostMigrationState(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	var duplicateCount int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 1 AND identity_key = 'account:acct-1'`).Scan(&duplicateCount).Error; err != nil {
		t.Fatalf("count duplicate rows: %v", err)
	}
	if duplicateCount != 1 {
		t.Fatalf("expected migrated duplicate identity rows to collapse to 1, got %d", duplicateCount)
	}

	var tenant2Count int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 2 AND identity_key = 'account:acct-1'`).Scan(&tenant2Count).Error; err != nil {
		t.Fatalf("count tenant 2 rows: %v", err)
	}
	if tenant2Count != 1 {
		t.Fatalf("expected tenant 2 migrated row to be preserved, got %d", tenant2Count)
	}

	var legacyEmailCount int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 1 AND identity_key = 'email:legacy@example.com'`).Scan(&legacyEmailCount).Error; err != nil {
		t.Fatalf("count legacy email rows: %v", err)
	}
	if legacyEmailCount != 1 {
		t.Fatalf("expected legacy null identity row to backfill to email identity, got %d", legacyEmailCount)
	}
}

func findMigrationByVersion(t *testing.T, version int) Migration {
	t.Helper()
	for _, migration := range migrations {
		if migration.Version == version {
			return migration
		}
	}
	t.Fatalf("migration v%d not found", version)
	return Migration{}
}

func assertIndexExists(t *testing.T, gormDB *gorm.DB, name string, wantUnique bool) {
	t.Helper()
	var rows []struct {
		Name   string `gorm:"column:name"`
		Unique int    `gorm:"column:unique"`
	}
	if err := gormDB.Raw(`PRAGMA index_list('codex_quotas')`).Scan(&rows).Error; err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	for _, row := range rows {
		if row.Name == name {
			if (row.Unique == 1) != wantUnique {
				t.Fatalf("index %s unique=%v, want %v", name, row.Unique == 1, wantUnique)
			}
			return
		}
	}
	t.Fatalf("expected index %s to exist; got %v", name, rows)
}

func assertIndexMissing(t *testing.T, gormDB *gorm.DB, name string) {
	t.Helper()
	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='%s'", name)
	if err := gormDB.Raw(query).Scan(&count).Error; err != nil {
		t.Fatalf("check index missing: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected index %s to be missing, got count %d", name, count)
	}
}
