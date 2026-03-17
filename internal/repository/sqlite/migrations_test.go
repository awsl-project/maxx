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
