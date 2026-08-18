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

// anthropicSlug is the one OpenRouter namespace whose maxx-native ids need the
// dash→dot version rewrite and datestamp strip below; every other vendor's ids
// are already in OpenRouter's form (see normalizeModelSlug).
const anthropicSlug = "anthropic/"

// vendorPrefixes maps a bare model-id prefix to the OpenRouter "vendor/" slug
// namespace it belongs to. Only vendors listed here are rewritten — an
// unrecognized id is left untouched so OpenRouter's own auto-resolution (or a
// clear upstream error) still applies exactly as before this normalization. The
// list is longest-prefix-first within each vendor where prefixes could overlap
// (ministral > mistral > mixtral), so the most specific vendor wins — though
// here the overlapping entries all map to the same slug, so ordering is only for
// the documented invariant, not correctness.
var vendorPrefixes = []struct {
	prefix string
	slug   string
}{
	{"claude", anthropicSlug},
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
	{"ministral", "mistralai/"},
	{"mistral", "mistralai/"},
	{"mixtral", "mistralai/"},
	{"codestral", "mistralai/"},
}

// datestampSuffix matches a trailing Anthropic-style release datestamp
// (e.g. "-20250514"), which OpenRouter slugs never carry. Applied to Anthropic
// ids only (OpenAI dated snapshots use 4-digit suffixes like -0613, not 8).
var datestampSuffix = regexp.MustCompile(`-\d{8}$`)

// versionDash matches a dash that separates two version digits — the "4-6" in
// claude-sonnet-4-6, which OpenRouter writes as a dot ("4.6"). This is applied to
// Anthropic ids ONLY: other vendors legitimately carry digit-dash-digit runs that
// are NOT version separators (gpt-4-32k, gpt-4-0613, qwen3-8b, gemma-3-4b-it,
// llama-3-70b), and rewriting those would corrupt them into non-existent slugs.
var versionDash = regexp.MustCompile(`(\d)-(\d)`)

// normalizeModelSlug rewrites a maxx/vendor-native model id into the
// "<vendor>/<model>" slug OpenRouter requires. It is a deliberate no-op for:
//   - ids that already contain "/" — an explicit slug from a ModelMapping entity
//     or the client always wins, so users retain a precise escape hatch;
//   - ids whose vendor it does not recognize — left untouched so nothing that
//     worked before (OpenRouter bare-name auto-resolution) regresses.
//
// For a recognized vendor it prepends the slug namespace. For Anthropic only it
// also strips a trailing release datestamp and converts version dashes to dots,
// so claude-sonnet-4-6 becomes anthropic/claude-sonnet-4.6 with no manual
// mapping. Non-Anthropic ids get the prefix and nothing else, because their
// digit-dash-digit runs (gpt-4-32k, qwen3-8b) are not version separators. The
// transform is idempotent on its own output.
func normalizeModelSlug(model string) string {
	m := strings.TrimSpace(model)
	if m == "" || strings.Contains(m, "/") {
		return model
	}
	low := strings.ToLower(m)
	for _, v := range vendorPrefixes {
		if !strings.HasPrefix(low, v.prefix) {
			continue
		}
		core := m
		if v.slug == anthropicSlug {
			core = datestampSuffix.ReplaceAllString(core, "")
			core = versionDash.ReplaceAllString(core, "$1.$2")
		}
		return v.slug + core
	}
	return model
}
