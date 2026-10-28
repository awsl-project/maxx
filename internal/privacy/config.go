package privacy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/privacy/pii"
	"github.com/awsl-project/maxx/internal/privacy/redact"
)

// Config is the project's complete privacy policy. It is deliberately owned by
// this module, rather than the router or the general project settings API.
type Config struct {
	Enabled         bool              `json:"enabled"`
	Secrets         bool              `json:"secrets"`
	Personal        bool              `json:"personal"`
	Kinds           map[string]bool   `json:"kinds,omitempty"`
	Words           []string          `json:"words,omitempty"`
	WordsAnyCase    bool              `json:"wordsAnyCase"`
	WordsWhole      bool              `json:"wordsWhole"`
	Rules           []redact.Rule     `json:"rules,omitempty"`
	Actions         map[string]string `json:"actions,omitempty"`
	Allow           []string          `json:"allow,omitempty"`
	Shape           bool              `json:"shape"`
	Mode            string            `json:"mode"` // mask, warn or block for kinds without an override
	Model           ModelConfig       `json:"model"`
	BypassProviders []uint64          `json:"bypassProviders,omitempty"`
}

type ModelConfig struct {
	Enabled bool    `json:"enabled"`
	Name    string  `json:"name"`
	Score   float64 `json:"score"` // zero uses the selected model's default
}

func DefaultConfig() Config {
	return Config{Secrets: true, Mode: "mask", Model: ModelConfig{Name: pii.AstrLinkGuard}}
}

// ValidateConfig normalizes copies, so a cached project or editor draft is never
// mutated. The rule compiler is Magpie's RE2 compiler with its original limits.
func ValidateConfig(in Config) (Config, error) {
	out := in
	if out.Mode == "" {
		out.Mode = "mask"
	}
	if !slices.Contains([]string{"mask", "warn", "block"}, out.Mode) {
		return Config{}, errors.New("privacy mode must be mask, warn or block")
	}
	var err error
	if out.Rules, err = redact.CheckRules(in.Rules); err != nil {
		return Config{}, err
	}
	if out.Words, err = normalizeValues(in.Words, 200, 256, in.WordsAnyCase); err != nil {
		return Config{}, fmt.Errorf("privacy words: %w", err)
	}
	if out.Allow, err = normalizeValues(in.Allow, 200, 256, true); err != nil {
		return Config{}, fmt.Errorf("privacy allowlist: %w", err)
	}
	out.Kinds = make(map[string]bool, len(in.Kinds))
	for id, enabled := range in.Kinds {
		if !slices.ContainsFunc(redact.Categories, func(c redact.Category) bool { return c.ID == id }) {
			return Config{}, fmt.Errorf("unknown privacy category %q", id)
		}
		out.Kinds[id] = enabled
	}
	out.Actions = make(map[string]string, len(in.Actions))
	for kind, action := range in.Actions {
		if !redact.KindName(kind) || !slices.Contains([]string{"mask", "warn", "block"}, action) {
			return Config{}, errors.New("privacy actions require an uppercase kind and mask, warn or block")
		}
		out.Actions[kind] = action
	}
	if out.Model.Name == "" {
		out.Model.Name = pii.AstrLinkGuard
	}
	if !slices.Contains([]string{pii.AstrLinkGuard, pii.PrivacyFilter}, out.Model.Name) {
		return Config{}, errors.New("unknown local privacy model")
	}
	if out.Model.Score != 0 && !(out.Model.Score >= pii.Floor && out.Model.Score < 1) {
		return Config{}, errors.New("privacy model score must be zero or at least 0.5 and less than 1")
	}
	if len(in.BypassProviders) > 200 {
		return Config{}, errors.New("at most 200 privacy provider exceptions")
	}
	out.BypassProviders = nil
	for _, id := range in.BypassProviders {
		if id == 0 {
			return Config{}, errors.New("privacy provider ID must be positive")
		}
		if !slices.Contains(out.BypassProviders, id) {
			out.BypassProviders = append(out.BypassProviders, id)
		}
	}
	return out, nil
}

func normalizeValues(in []string, count, width int, fold bool) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > width {
			return nil, fmt.Errorf("each value must contain at most %d Unicode characters", width)
		}
		key := value
		if fold {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	if len(out) > count {
		return nil, fmt.Errorf("at most %d values", count)
	}
	return out, nil
}

func projectConfig(project *domain.Project) (Config, error) {
	if len(project.Privacy) == 0 || bytes.Equal(project.Privacy, []byte("null")) {
		cfg := DefaultConfig()
		cfg.Enabled = project.BlockSecrets
		// Existing callers explicitly requested credential blocking. They keep
		// that behavior until the dedicated settings page saves a full policy.
		if project.BlockSecrets {
			cfg.Mode = "block"
		}
		return cfg, nil
	}
	var cfg Config
	d := json.NewDecoder(bytes.NewReader(project.Privacy))
	d.DisallowUnknownFields()
	if err := d.Decode(&cfg); err != nil {
		return Config{}, errors.New("invalid stored privacy settings")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return Config{}, errors.New("invalid stored privacy settings")
	}
	return ValidateConfig(cfg)
}

func (s *Service) Configuration(tenantID, projectID uint64) (Config, error) {
	if s == nil || s.projects == nil {
		return Config{}, domain.ErrInvalidState
	}
	project, err := s.projects.GetByID(tenantID, projectID)
	if err != nil {
		return Config{}, err
	}
	if project == nil {
		return Config{}, domain.ErrNotFound
	}
	return projectConfig(project)
}

func (s *Service) SetConfiguration(tenantID, projectID uint64, config Config) (*domain.Project, error) {
	if s == nil || s.projects == nil {
		return nil, domain.ErrInvalidState
	}
	cfg, err := ValidateConfig(config)
	if err != nil {
		return nil, err
	}
	project, err := s.projects.GetByID(tenantID, projectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, domain.ErrNotFound
	}
	updated := *project
	updated.Privacy, err = json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	updated.BlockSecrets = cfg.Enabled
	if err := s.projects.Update(&updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// SyncLegacyProtection keeps the legacy boolean API compatible without
// discarding the rest of a project's new privacy configuration.
func SyncLegacyProtection(project *domain.Project) error {
	if len(project.Privacy) == 0 {
		return nil
	}
	cfg, err := projectConfig(project)
	if err != nil {
		return err
	}
	cfg.Enabled = project.BlockSecrets
	project.Privacy, err = json.Marshal(cfg)
	return err
}
