package bedrock

import (
	"regexp"
	"strings"
)

// aliasMapping maps short/CLI model aliases to their full Anthropic names.
// Only aliases that can't be auto-resolved need to be here.
var aliasMapping = map[string]string{
	// Claude Code CLI uses shortened names without date suffix
	"claude-sonnet-4-6":   "claude-sonnet-4-20250514",
	"claude-sonnet-4-5":   "claude-sonnet-4-5-20250514",
	"claude-opus-4-6":     "claude-opus-4-20250514",
	"claude-haiku-4-5":    "claude-haiku-4-20250514",
	"claude-sonnet-4":     "claude-sonnet-4-20250514",
	"claude-opus-4":       "claude-opus-4-20250514",
	"claude-3-5-sonnet":   "claude-3-5-sonnet-20241022",
	"claude-3-5-haiku":    "claude-3-5-haiku-20241022",
}

// versionOverrides maps model names to specific Bedrock version suffixes
// when the version isn't simply "v1:0".
var versionOverrides = map[string]string{
	"claude-3-5-sonnet-20241022": "v2:0",
}

// modelDatePattern matches Anthropic model names like "claude-sonnet-4-20250514"
var modelDatePattern = regexp.MustCompile(`^claude-[\w-]+-\d{8}$`)

// resolveModelID maps a request model name to a Bedrock model ID.
// Priority: user config mapping > alias resolution > auto-derive > passthrough.
func resolveModelID(requestModel string, configMapping map[string]string, modelPrefix string) string {
	// 1. Check user-configured mapping (highest priority)
	if configMapping != nil {
		if mapped, ok := configMapping[requestModel]; ok {
			return applyPrefix(mapped, modelPrefix)
		}
	}

	// 2. Resolve alias to full Anthropic name
	model := requestModel
	if alias, ok := aliasMapping[model]; ok {
		model = alias
	}

	// 3. Auto-derive Bedrock model ID from Anthropic name
	// Pattern: "claude-xxx-YYYYMMDD" -> "anthropic.claude-xxx-YYYYMMDD-v1:0"
	if modelDatePattern.MatchString(model) {
		version := "v1:0"
		if v, ok := versionOverrides[model]; ok {
			version = v
		}
		bedrockID := "anthropic." + model + "-" + version
		return applyPrefix(bedrockID, modelPrefix)
	}

	// 4. Already a Bedrock model ID or unknown format — passthrough
	return applyPrefix(model, modelPrefix)
}

// applyPrefix adds the region prefix (e.g., "us.") if the model ID doesn't already have one
// and the prefix is non-empty.
func applyPrefix(modelID string, prefix string) string {
	if prefix == "" {
		return modelID
	}
	// Already has a region prefix (e.g., "us.anthropic.claude-...")
	if strings.Contains(modelID, ".") && !strings.HasPrefix(modelID, "anthropic.") {
		return modelID
	}
	return prefix + "." + modelID
}
