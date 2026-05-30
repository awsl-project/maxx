package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/cmd/maxx-cli/internal/cfg"
)

// dryRunFixture sets up an isolated config file at path and seeds it with
// one context named "test". It restores the global flag and config-path
// state via t.Cleanup.
func dryRunFixture(t *testing.T) (configPath string) {
	t.Helper()
	dir := t.TempDir()
	configPath = filepath.Join(dir, "config.yaml")
	t.Setenv("MAXX_CLI_CONFIG", configPath)
	conf := &cfg.Config{
		CurrentContext: "test",
		Contexts: []*cfg.Context{
			{Name: "test", Server: "http://localhost:9880", Token: "fake-token"},
		},
	}
	if err := cfg.Save(conf); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	prev := flagDryRun
	flagDryRun = true
	t.Cleanup(func() { flagDryRun = prev })
	return configPath
}

// readConfig returns the on-disk config, used to assert no mutation
// happened during a dry-run.
func readConfig(t *testing.T) *cfg.Config {
	t.Helper()
	c, err := cfg.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

func TestDryRunContextUseDoesNotMutate(t *testing.T) {
	dryRunFixture(t)
	before := readConfig(t)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--dry-run", "context", "use", "test"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "[dry-run] would switch current context") {
		t.Errorf("output missing dry-run preview: %q", out.String())
	}
	after := readConfig(t)
	if after.CurrentContext != before.CurrentContext {
		t.Errorf("dry-run changed CurrentContext: before=%q after=%q", before.CurrentContext, after.CurrentContext)
	}
	if len(after.Contexts) != len(before.Contexts) {
		t.Errorf("dry-run changed context count: before=%d after=%d", len(before.Contexts), len(after.Contexts))
	}
}

func TestDryRunContextDeleteDoesNotMutate(t *testing.T) {
	dryRunFixture(t)
	before := readConfig(t)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--dry-run", "context", "delete", "test"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "[dry-run] would delete context") {
		t.Errorf("output missing dry-run preview: %q", out.String())
	}
	after := readConfig(t)
	if len(after.Contexts) != len(before.Contexts) {
		t.Errorf("dry-run mutated config: before=%d after=%d", len(before.Contexts), len(after.Contexts))
	}
	if after.CurrentContext != before.CurrentContext {
		t.Errorf("dry-run cleared CurrentContext")
	}
}

func TestDryRunLogoutDoesNotMutate(t *testing.T) {
	dryRunFixture(t)
	before := readConfig(t)
	beforeTok := before.Contexts[0].Token

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--dry-run", "logout"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "[dry-run] would clear token") {
		t.Errorf("output missing dry-run preview: %q", out.String())
	}
	after := readConfig(t)
	if after.Contexts[0].Token != beforeTok {
		t.Errorf("dry-run mutated token: before=%q after=%q", beforeTok, after.Contexts[0].Token)
	}
}

func TestDryRunLoginDoesNotContactOrPersist(t *testing.T) {
	configPath := dryRunFixture(t)
	// Snapshot the file's mtime so we can detect any write.
	beforeStat, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	cmd := newRootCmd()
	// Point at a definitely-closed port so any real network attempt would
	// surface as a connection-refused error in the test output.
	cmd.SetArgs([]string{
		"--dry-run", "login",
		"--server", "http://127.0.0.1:1",
		"--username", "alice",
		"--password", "irrelevant",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "[dry-run] POST") {
		t.Errorf("output missing dry-run POST preview: %q", out.String())
	}
	if strings.Contains(out.String(), "connection refused") {
		t.Errorf("dry-run still attempted network: %q", out.String())
	}

	afterStat, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !afterStat.ModTime().Equal(beforeStat.ModTime()) {
		t.Errorf("dry-run login modified the config file")
	}
}

func TestDryRunStrategyStickyDoesNotContactOrRequireLogin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MAXX_CLI_CONFIG", filepath.Join(dir, "missing.yaml"))
	prev := flagDryRun
	flagDryRun = true
	t.Cleanup(func() { flagDryRun = prev })

	// Note: no context configured at all. A non-dry-run would error with
	// "no current context"; a buggy dry-run that runs authedClient first
	// would error the same way. The fix must short-circuit before auth.
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--dry-run", "strategy", "sticky", "7", "on", "--scope", "conversation", "--ttl", "900"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "[dry-run] would toggle sticky=on for routing strategy 7") {
		t.Errorf("output missing dry-run preview: %q", out.String())
	}
	if !strings.Contains(out.String(), "scope=conversation") || !strings.Contains(out.String(), "ttlSeconds=900") {
		t.Errorf("output missing scope/ttl details: %q", out.String())
	}
}
