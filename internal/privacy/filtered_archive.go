package privacy

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/awsl-project/maxx/internal/privacy/redact"
)

// Archive filters already credential-scrubbed copies. It never runs inference
// in a database write. Model findings, including warn-only findings, were
// captured during inspection and are removed irreversibly here.
func (e *Exchange) Archive(text string) string {
	if e == nil || !e.Enabled || text == "" {
		return text
	}
	if e.failed || !e.evaluated || e.Bypassed && e.Config.Model.Enabled {
		return "[REDACTED: privacy inspection did not run or complete]"
	}
	scope := e.scope
	if scope == nil {
		scope = e.runtime.scope(e.key)
	}
	options := redact.Options{Secrets: true, Personal: e.Config.Personal, Kinds: e.Config.Kinds,
		Words: e.Config.Words, WordsAnyCase: e.Config.WordsAnyCase, WordsWhole: e.Config.WordsWhole, Rules: e.Config.Rules}
	values := make([]string, 0, len(e.matched))
	for value := range e.matched {
		if value != "" {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	var pairs []string
	for _, value := range values {
		pairs = append(pairs, value, Redacted)
	}
	replacer := strings.NewReplacer(pairs...)
	cleanText := func(text string) string {
		cleaned, _ := scope.state.RedactText(replacer.Replace(text), options)
		return cleaned
	}
	var walk func(any) any
	walk = func(value any) any {
		switch v := value.(type) {
		case map[string]any:
			out := make(map[string]any, len(v))
			for key, child := range v {
				out[cleanText(key)] = walk(child)
			}
			return out
		case []any:
			for i := range v {
				v[i] = walk(v[i])
			}
		case string:
			if mediaDataURL.MatchString(v) {
				return v
			}
			if nested, err := decode(v); err == nil {
				switch nested.(type) {
				case map[string]any, []any:
					encoded, _ := json.Marshal(walk(nested))
					return string(encoded)
				}
			}
			return cleanText(v)
		}
		return value
	}
	if value, err := decode(text); err == nil {
		cleaned, _ := json.Marshal(walk(value))
		return string(cleaned)
	}
	lines := strings.Split(text, "\n")
	var combined strings.Builder
	frames := 0
	for i, line := range lines {
		prefix, payload := "", strings.TrimSpace(line)
		if strings.HasPrefix(payload, "data:") {
			prefix, payload = "data: ", strings.TrimSpace(strings.TrimPrefix(payload, "data:"))
		}
		if value, err := decode(payload); err == nil {
			frames++
			collectText(value, &combined)
			cleaned, _ := json.Marshal(walk(value))
			lines[i] = prefix + string(cleaned)
		} else {
			lines[i] = cleanText(line)
		}
	}
	if frames > 0 && cleanText(combined.String()) != combined.String() {
		return "[REDACTED: private content in streamed response]"
	}
	return strings.Join(lines, "\n")
}
