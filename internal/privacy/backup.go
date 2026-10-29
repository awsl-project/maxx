package privacy

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/awsl-project/maxx/internal/domain"
)

type backupConfig struct {
	Config
	BypassProviderNames []string `json:"bypassProviderNames,omitempty"`
}

// ExportPolicy uses provider names, matching Maxx's existing backup references.
// Database IDs must never silently exempt an unrelated provider after restore.
func ExportPolicy(raw json.RawMessage, names map[uint64]string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	cfg, err := ParseConfig(string(raw))
	if err != nil {
		return nil, err
	}
	out := backupConfig{Config: cfg}
	for _, id := range cfg.BypassProviders {
		if name := names[id]; name != "" {
			out.BypassProviderNames = append(out.BypassProviderNames, name)
		}
	}
	out.BypassProviders = nil
	return json.Marshal(out)
}
func ImportPolicy(raw json.RawMessage, ids map[string]uint64) (json.RawMessage, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var in backupConfig
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, domain.ErrInvalidInput
	}
	// Drop raw source IDs, including old snapshots. An unresolved name removes
	// the exception, retaining protection rather than trusting a stale ID.
	in.BypassProviders = nil
	for _, name := range in.BypassProviderNames {
		if id := ids[name]; id != 0 {
			in.BypassProviders = append(in.BypassProviders, id)
		}
	}
	cfg, err := ValidateConfig(in.Config)
	if err != nil {
		return nil, err
	}
	return json.Marshal(cfg)
}
