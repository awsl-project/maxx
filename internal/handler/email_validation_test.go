package handler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeAndValidateRegistrationEmail(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantEmail string
		wantErr   error
	}{
		{
			name:      "valid email",
			input:     " User@example.com ",
			wantEmail: "user@example.com",
		},
		{
			name:    "invalid email",
			input:   "not-email",
			wantErr: errInvalidEmail,
		},
		{
			name:    "disposable email",
			input:   "foo@mailinator.com",
			wantErr: errDisposableEmail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeAndValidateRegistrationEmail(tt.input)
			if tt.wantErr != nil {
				if err == nil || err.Error() != tt.wantErr.Error() {
					t.Fatalf("expected error %q, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantEmail {
				t.Fatalf("expected normalized email %q, got %q", tt.wantEmail, got)
			}
		})
	}
}

func TestReloadDisposableDomains_FromFileAndFallback(t *testing.T) {
	original := cloneDomainSet(disposableEmailDomains)
	defer setDisposableEmailDomains(original)

	tempFile := filepath.Join(t.TempDir(), "domains.txt")
	content := "custom-temp.test\n# comment\nanother-temp.test\n"
	if err := os.WriteFile(tempFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp domain file: %v", err)
	}

	t.Setenv(disposableEmailDomainsFileEnvKey, tempFile)
	if err := ReloadDisposableDomains(); err != nil {
		t.Fatalf("expected reload from file to succeed, got: %v", err)
	}

	if !isDisposableEmailDomain("foo.custom-temp.test") {
		t.Fatalf("expected custom domain from file to be blocked")
	}

	t.Setenv(disposableEmailDomainsFileEnvKey, filepath.Join(t.TempDir(), "missing.txt"))
	if err := ReloadDisposableDomains(); err == nil {
		t.Fatalf("expected reload to fail when file is missing")
	}

	if !isDisposableEmailDomain("mailinator.com") {
		t.Fatalf("expected bundled default domain to remain blocked after failed reload")
	}
	if isDisposableEmailDomain("custom-temp.test") {
		t.Fatalf("expected custom domain to be dropped after fallback to bundled defaults")
	}
}
