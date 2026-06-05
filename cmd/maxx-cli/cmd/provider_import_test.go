package cmd

import (
	"testing"
)

func TestNormaliseProviderImportInputArray(t *testing.T) {
	input := []byte(`[{"id":1,"name":"a","type":"custom"}, {"id":2,"name":"b","type":"custom"}]`)
	got, err := normaliseProviderImportInput(input)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Name != "a" || got[1].Name != "b" {
		t.Errorf("names = %q, %q", got[0].Name, got[1].Name)
	}
}

func TestNormaliseProviderImportInputSingle(t *testing.T) {
	input := []byte(`{"id":1,"name":"only","type":"custom"}`)
	got, err := normaliseProviderImportInput(input)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Name != "only" {
		t.Errorf("name = %q", got[0].Name)
	}
}

func TestNormaliseProviderImportInputRejectsScalar(t *testing.T) {
	if _, err := normaliseProviderImportInput([]byte(`"nope"`)); err == nil {
		t.Error("expected error for scalar input")
	}
	if _, err := normaliseProviderImportInput([]byte(`   `)); err == nil {
		t.Error("expected error for empty input")
	}
}

func TestNormaliseProviderImportInputTolerantWhitespace(t *testing.T) {
	input := []byte("\n\n  [ ]  ")
	got, err := normaliseProviderImportInput(input)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestNormaliseProviderImportInputStripsUTF8BOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	input := append(bom, []byte(`{"id":1,"name":"bom","type":"custom"}`)...)
	got, err := normaliseProviderImportInput(input)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 || got[0].Name != "bom" {
		t.Errorf("got = %+v, want one provider named 'bom'", got)
	}
}
