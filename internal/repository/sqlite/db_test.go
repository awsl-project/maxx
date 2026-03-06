package sqlite

import (
	"testing"
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
