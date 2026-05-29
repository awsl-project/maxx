// Package cfg loads/saves the maxx-cli config file at ~/.config/maxx-cli/config.yaml
// (XDG_CONFIG_HOME aware), with kubectl-style contexts.
package cfg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk shape.
type Config struct {
	CurrentContext string     `yaml:"currentContext"`
	Contexts       []*Context `yaml:"contexts"`
}

// Context holds one server endpoint + credentials.
type Context struct {
	Name      string    `yaml:"name"`
	Server    string    `yaml:"server"`
	Token     string    `yaml:"token,omitempty"`
	ExpiresAt time.Time `yaml:"expiresAt,omitempty"`
	Username  string    `yaml:"username,omitempty"`

	// InsecureSkipVerify lets the CLI talk to a server with a self-signed cert.
	InsecureSkipVerify bool `yaml:"insecureSkipVerify,omitempty"`
}

// ErrNoContext is returned when the current context is missing or unset.
var ErrNoContext = errors.New("no current context; run `maxx-cli login` first")

// Path returns the resolved config path, honouring XDG_CONFIG_HOME and
// MAXX_CLI_CONFIG override.
func Path() (string, error) {
	if override := os.Getenv("MAXX_CLI_CONFIG"); override != "" {
		return override, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "maxx-cli", "config.yaml"), nil
}

// legacyPath is the pre-rename location used by an early draft of the CLI.
func legacyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "maxx", "cli.yaml"), nil
}

// Load reads the config. If the new path is missing but a legacy file exists,
// it migrates once and returns the migrated config.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if migrated, ok, mErr := tryMigrate(path); ok {
			return migrated, mErr
		}
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &c, nil
}

// tryMigrate copies a legacy config into the new path. Returns the loaded
// config on success; (nil,false,nil) when no legacy file exists.
func tryMigrate(newPath string) (*Config, bool, error) {
	old, err := legacyPath()
	if err != nil {
		return nil, false, nil
	}
	data, err := os.ReadFile(old)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read legacy config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, false, fmt.Errorf("parse legacy config: %w", err)
	}
	if err := saveTo(newPath, &c); err != nil {
		return nil, false, fmt.Errorf("write migrated config: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[maxx-cli] migrated config from %s to %s\n", old, newPath)
	return &c, true, nil
}

// Save writes the config to disk with 0600 permissions and 0700 on the dir.
func Save(c *Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return saveTo(path, c)
}

func saveTo(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}
	// Belt-and-suspenders: enforce 0600 even on filesystems that ignore the
	// initial WriteFile mode (e.g. some NFS mounts).
	_ = os.Chmod(path, 0o600)
	return nil
}

// FindContext returns the context with the given name, or nil if absent.
func (c *Config) FindContext(name string) *Context {
	for _, ctx := range c.Contexts {
		if ctx.Name == name {
			return ctx
		}
	}
	return nil
}

// UpsertContext adds or replaces a context in-place.
func (c *Context) Validate() error {
	if c.Name == "" {
		return errors.New("context name is required")
	}
	if c.Server == "" {
		return errors.New("context server URL is required")
	}
	return nil
}

// Upsert inserts or replaces ctx by name. Does not change CurrentContext.
func (c *Config) Upsert(ctx *Context) {
	for i, existing := range c.Contexts {
		if existing.Name == ctx.Name {
			c.Contexts[i] = ctx
			return
		}
	}
	c.Contexts = append(c.Contexts, ctx)
}

// Remove deletes the context by name. Returns true if a context was removed.
// If the removed context was current, CurrentContext is cleared.
func (c *Config) Remove(name string) bool {
	for i, existing := range c.Contexts {
		if existing.Name == name {
			c.Contexts = append(c.Contexts[:i], c.Contexts[i+1:]...)
			if c.CurrentContext == name {
				c.CurrentContext = ""
			}
			return true
		}
	}
	return false
}

// Current returns the active context or ErrNoContext.
func (c *Config) Current() (*Context, error) {
	if c.CurrentContext == "" {
		return nil, ErrNoContext
	}
	ctx := c.FindContext(c.CurrentContext)
	if ctx == nil {
		return nil, fmt.Errorf("current context %q not found in config", c.CurrentContext)
	}
	return ctx, nil
}
