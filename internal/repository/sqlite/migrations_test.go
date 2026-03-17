package sqlite

import (
	"errors"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
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
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id) VALUES (1, 'account:acct-1', 'first@example.com', 'acct-1')`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id) VALUES (1, 'account:acct-1', 'second@example.com', 'acct-1')`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id) VALUES (1, 'account:acct-2', 'third@example.com', 'acct-2')`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id) VALUES (2, 'account:acct-1', 'other-tenant@example.com', 'acct-1')`,
		`INSERT INTO codex_quotas (tenant_id, identity_key, email, account_id) VALUES (1, NULL, 'legacy@example.com', '')`,
	}
	for _, sql := range inserts {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}

	if err := dedupeCodexQuotaIdentityRows(gormDB); err != nil {
		t.Fatalf("dedupe identities: %v", err)
	}

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
