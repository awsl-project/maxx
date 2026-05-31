package cfg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MAXX_CLI_CONFIG", filepath.Join(dir, "config.yaml"))

	c := &Config{
		CurrentContext: "prod",
		Contexts: []*Context{
			{Name: "prod", Server: "https://api.example.com", Token: "abc", Username: "alice"},
			{Name: "local", Server: "http://localhost:9880"},
		},
	}
	if err := Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.CurrentContext != "prod" {
		t.Errorf("CurrentContext = %q, want %q", got.CurrentContext, "prod")
	}
	if len(got.Contexts) != 2 {
		t.Errorf("len(Contexts) = %d, want 2", len(got.Contexts))
	}
	if cur, err := got.Current(); err != nil {
		t.Errorf("Current: %v", err)
	} else if cur.Token != "abc" {
		t.Errorf("token mismatch: %q", cur.Token)
	}
}

func TestSavePermissionsAre0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	t.Setenv("MAXX_CLI_CONFIG", path)

	c := &Config{Contexts: []*Context{{Name: "a", Server: "http://x"}}}
	if err := Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %#o, want 0600", got)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MAXX_CLI_CONFIG", filepath.Join(dir, "missing.yaml"))
	// Override HOME so legacyPath() resolves to a non-existent location.
	t.Setenv("HOME", dir)

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.CurrentContext != "" || len(got.Contexts) != 0 {
		t.Errorf("expected empty config, got %+v", got)
	}
}

func TestRemoveClearsCurrent(t *testing.T) {
	c := &Config{
		CurrentContext: "a",
		Contexts: []*Context{
			{Name: "a", Server: "http://x"},
			{Name: "b", Server: "http://y"},
		},
	}
	if !c.Remove("a") {
		t.Fatal("Remove returned false")
	}
	if c.CurrentContext != "" {
		t.Errorf("CurrentContext = %q, want empty", c.CurrentContext)
	}
	if len(c.Contexts) != 1 || c.Contexts[0].Name != "b" {
		t.Errorf("contexts after remove: %+v", c.Contexts)
	}
}

func TestUpsertReplacesInPlace(t *testing.T) {
	c := &Config{Contexts: []*Context{{Name: "a", Server: "http://x"}}}
	c.Upsert(&Context{Name: "a", Server: "http://y"})
	if len(c.Contexts) != 1 {
		t.Fatalf("len = %d, want 1", len(c.Contexts))
	}
	if c.Contexts[0].Server != "http://y" {
		t.Errorf("server = %q, want http://y", c.Contexts[0].Server)
	}
	c.Upsert(&Context{Name: "b", Server: "http://z"})
	if len(c.Contexts) != 2 {
		t.Errorf("len after append = %d, want 2", len(c.Contexts))
	}
}

func TestLegacyMigration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// New path goes into XDG_CONFIG_HOME so it can't collide with HOME/.config/maxx.
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("MAXX_CLI_CONFIG", "") // unset override so we exercise the resolver

	legacyDir := filepath.Join(home, ".config", "maxx")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacyContent := `currentContext: legacy
contexts:
  - name: legacy
    server: http://legacy.example.com
    token: legtok
`
	if err := os.WriteFile(filepath.Join(legacyDir, "cli.yaml"), []byte(legacyContent), 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.CurrentContext != "legacy" {
		t.Errorf("CurrentContext = %q after migration, want %q", got.CurrentContext, "legacy")
	}
	// And verify the new path now exists.
	newPath := filepath.Join(xdg, "maxx-cli", "config.yaml")
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected migrated file at %s, stat err %v", newPath, err)
	}
}
