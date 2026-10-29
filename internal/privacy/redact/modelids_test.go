package redact

import (
	"strings"
	"testing"
)

// The local model took the ids in Codex's turn metadata for secrets, and
// each went as {{SECRET_…}}: the installation's, the session's, the
// turn's, the window's (#1977, the owner's records). It does the same to a
// request id in a log and a digest named by its algorithm; those are what
// it labelled "secret" when run on these texts. They go as written, in
// client_metadata and anywhere else. A password and a key it finds are
// still masked.
func TestModelSecretIsNoID(t *testing.T) {
	meta := `{"installation_id":"6f1c2a0e-8b7d-4c3a-9e21-5d4b3c2a1f00","session_id":"019a0b1c-2d3e-7f40-8a1b-2c3d4e5f6a7b","agent_name":"/root","turn_id":"019a0b1c-9999-7f40-8a1b-2c3d4e5f6a7b","window_id":"019a0b1c-2d3e-7f40-8a1b-2c3d4e5f6a7b:0","window_number":0,"request_kind":"turn"}`
	f := &fakeModel{values: map[string]string{
		"6f1c2a0e-8b7d-4c3a-9e21-5d4b3c2a1f00":                                    "secret", // gitleaks:allow -- public upstream regression fixture or detector example
		"019a0b1c-2d3e-7f40-8a1b-2c3d4e5f6a7b":                                    "secret", // gitleaks:allow -- public upstream regression fixture or detector example
		"019a0b1c-9999-7f40-8a1b-2c3d4e5f6a7b":                                    "secret",
		"019a0b1c-2d3e-7f40-8a1b-2c3d4e5f6a7b:0":                                  "secret", // gitleaks:allow -- public upstream regression fixture or detector example
		"4f2a9c1e-77b3-4d1a-b2e0-9c8d7e6f5a4b":                                    "secret",
		"sha256 9b74c9897bac770ffc029102a200c5de2b6e0f3c7f1d0f2a7c3b5e1d9f8a7c6b": "secret",
		"/Users/vdxz/.codex/skills/wrangler/SKILL.md":                             "secret",
		"Tr0ub4dor&3xq":                    "secret", // gitleaks:allow -- public upstream regression fixture or detector example
		"9f8e7d6c5b4a39281706f5e4d3c2b1a0": "secret",
	}}
	o := Options{Secrets: true, Personal: true, Model: f.find}

	body := `{"model":"gpt-5.5","client_metadata":{"x-codex-turn-metadata":` + quoteJSON(meta) + `},"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"request id 4f2a9c1e-77b3-4d1a-b2e0-9c8d7e6f5a4b failed; sha256 9b74c9897bac770ffc029102a200c5de2b6e0f3c7f1d0f2a7c3b5e1d9f8a7c6b; - wrangler: Cloudflare Workers CLI (file: /Users/vdxz/.codex/skills/wrangler/SKILL.md). my password is Tr0ub4dor&3xq and the api key is 9f8e7d6c5b4a39281706f5e4d3c2b1a0"}]}]}`
	out, n := MaskJSON([]byte(body), o)
	got := string(out)
	if n != 2 || strings.Contains(got, "Tr0ub4dor&3xq") || strings.Contains(got, "9f8e7d6c5b4a39281706f5e4d3c2b1a0") {
		t.Fatalf("%d masked, want the password and the key: %s", n, got)
	}
	for _, v := range []string{quoteJSON(meta), "request id 4f2a9c1e-77b3-4d1a-b2e0-9c8d7e6f5a4b failed", "sha256 9b74c9897bac", "(file: /Users/vdxz/.codex/skills/wrangler/SKILL.md)"} {
		if !strings.Contains(got, v) {
			t.Errorf("%s went masked: %s", v, got)
		}
	}

	// a word the user masks is kept in client_metadata too: no model reads it
	w := `{"client_metadata":{"x-codex-turn-metadata":"{\"agent_name\":\"/atlas\"}"},"input":"ship atlas"}`
	if out, n := MaskJSON([]byte(w), Options{Words: []string{"atlas"}}); n != 1 || !strings.Contains(string(out), `/atlas`) {
		t.Fatalf("%d masked: %s", n, out)
	}
}

func quoteJSON(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
