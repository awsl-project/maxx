package openrouter

import (
	"regexp"
	"strings"
)

// OpenRouter addresses every model as "<vendor>/<model>" (e.g.
// anthropic/claude-sonnet-4.6), whereas maxx clients send the vendor-native id
// (claude-sonnet-4-6). Without the vendor namespace OpenRouter rejects the
// request outright ("... is not a valid model ID"), and its own bare-name
// auto-resolution does NOT cover Anthropic's dash-versioned ids. Rather than make
// every user hand-author a ModelMapping per model, the adapter derives the slug
// automatically. See normalizeModelSlug.

// vendorPrefixes maps a bare model-id prefix to the OpenRouter "vendor/" slug
// namespace it belongs to. Only vendors listed here are rewritten — an
// unrecognized id is left untouched so OpenRouter's own auto-resolution (or a
// clear upstream error) still applies exactly as before this normalization. The
// list is longest-prefix-first where prefixes could overlap, so the most specific
// vendor wins.
var vendorPrefixes = []struct {
	prefix string
	slug   string
}{
	{"claude", "anthropic/"},
	{"chatgpt", "openai/"},
	{"codex", "openai/"},
	{"gpt", "openai/"},
	{"o1", "openai/"},
	{"o3", "openai/"},
	{"o4", "openai/"},
	{"gemini", "google/"},
	{"gemma", "google/"},
	{"grok", "x-ai/"},
	{"deepseek", "deepseek/"},
	{"qwen", "qwen/"},
	{"llama", "meta-llama/"},
	{"mixtral", "mistralai/"},
	{"mistral", "mistralai/"},
	{"ministral", "mistralai/"},
	{"codestral", "mistralai/"},
}

// datestampSuffix matches a trailing Anthropic-style release datestamp
// (e.g. "-20250514"), which OpenRouter slugs never carry.
var datestampSuffix = regexp.MustCompile(`-\d{8}$`)

// versionDash matches a dash that separates two version digits — the "4-6" in
// claude-sonnet-4-6, which OpenRouter writes as a dot ("4.6"). A dash between a
// letter and a digit (the "sonnet-4" boundary) is a name separator and is left
// alone.
var versionDash = regexp.MustCompile(`(\d)-(\d)`)

// normalizeModelSlug rewrites a maxx/vendor-native model id into the
// "<vendor>/<model>" slug OpenRouter requires. It is a deliberate no-op for:
//   - ids that already contain "/" — an explicit slug from a ModelMapping entity
//     or the client always wins, so users retain a precise escape hatch;
//   - ids whose vendor it does not recognize — left untouched so nothing that
//     worked before (OpenRouter bare-name auto-resolution) regresses.
//
// For a recognized vendor it prepends the slug namespace, strips a trailing
// release datestamp, and converts version dashes to dots, so claude-sonnet-4-6
// becomes anthropic/claude-sonnet-4.6 with no manual mapping. The transform is
// idempotent on its own output.
func normalizeModelSlug(model string) string {
	m := strings.TrimSpace(model)
	if m == "" || strings.Contains(m, "/") {
		return model
	}
	low := strings.ToLower(m)
	for _, v := range vendorPrefixes {
		if strings.HasPrefix(low, v.prefix) {
			core := datestampSuffix.ReplaceAllString(m, "")
			core = versionDash.ReplaceAllString(core, "$1.$2")
			return v.slug + core
		}
	}
	return model
}
