package responsemodifier

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

var claudeSSEDataLineRE = regexp.MustCompile(`(?m)^(\s*data:\s*)(.*?)(\r?\n?)$`)

type claudeResponseModifier struct {
	mapping  map[string]string
	patterns []string
}

func newClaudeResponseModifier(provider *domain.Provider, clientType domain.ClientType) *claudeResponseModifier {
	if clientType != domain.ClientTypeClaude || provider.Config == nil {
		return nil
	}
	mapping := responseModelMapping(provider.Config)
	if len(mapping) == 0 {
		return nil
	}
	return &claudeResponseModifier{mapping: mapping, patterns: sortedMappingPatterns(mapping)}
}

func responseModelMapping(config *domain.ProviderConfig) map[string]string {
	if config.Custom != nil && len(config.Custom.ResponseModelMapping) > 0 {
		return config.Custom.ResponseModelMapping
	}
	if config.Claude != nil {
		return config.Claude.ResponseModelMapping
	}
	return nil
}

func (m *claudeResponseModifier) modifyBody(body []byte) []byte {
	return m.rewriteJSON(body)
}

func (m *claudeResponseModifier) modifyStreamEvent(body []byte) []byte {
	return claudeSSEDataLineRE.ReplaceAllFunc(body, func(line []byte) []byte {
		parts := claudeSSEDataLineRE.FindSubmatch(line)
		if len(parts) != 4 || bytes.Equal(bytes.TrimSpace(parts[2]), []byte("[DONE]")) {
			return line
		}
		payload := m.rewriteJSON(parts[2])
		if bytes.Equal(payload, parts[2]) {
			return line
		}
		out := make([]byte, 0, len(parts[1])+len(payload)+len(parts[3]))
		out = append(out, parts[1]...)
		out = append(out, payload...)
		out = append(out, parts[3]...)
		return out
	})
}

func (m *claudeResponseModifier) rewriteJSON(body []byte) []byte {
	if len(bytes.TrimSpace(body)) == 0 {
		return body
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		return body
	}
	changed := m.rewriteModel(object, "model")
	if message, ok := object["message"].(map[string]any); ok {
		changed = m.rewriteModel(message, "model") || changed
	}
	if !changed {
		return body
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(object); err != nil {
		return body
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

func (m *claudeResponseModifier) rewriteModel(object map[string]any, key string) bool {
	value, ok := object[key].(string)
	if !ok {
		return false
	}
	mapped := m.mapModel(value)
	if mapped == value {
		return false
	}
	object[key] = mapped
	return true
}

func (m *claudeResponseModifier) mapModel(model string) string {
	model = strings.TrimSpace(model)
	patterns := m.patterns
	if patterns == nil {
		patterns = sortedMappingPatterns(m.mapping)
	}
	for _, pattern := range patterns {
		target := strings.TrimSpace(m.mapping[pattern])
		if target == "" || strings.Contains(target, "*") {
			continue
		}
		if domain.MatchWildcard(pattern, model) {
			return target
		}
	}
	return model
}

func sortedMappingPatterns(mapping map[string]string) []string {
	patterns := make([]string, 0, len(mapping))
	for pattern := range mapping {
		patterns = append(patterns, pattern)
	}
	sort.SliceStable(patterns, func(i, j int) bool {
		left, right := patterns[i], patterns[j]
		leftWildcards, rightWildcards := strings.Count(left, "*"), strings.Count(right, "*")
		if leftWildcards != rightWildcards {
			return leftWildcards < rightWildcards
		}
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		return left < right
	})
	return patterns
}
