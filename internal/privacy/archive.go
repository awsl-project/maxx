package privacy

import (
	"encoding/json"
	"github.com/awsl-project/maxx/internal/domain"
	"net/url"
	"regexp"
	"strings"
)

var mediaDataURL = regexp.MustCompile(`(?i)^data:(?:image|audio|video)/[A-Za-z0-9.+_-]+(?:;[A-Za-z0-9._=+-]+)*;base64,[A-Za-z0-9+/]*={0,2}$`)

func scrub(value any) any {
	switch v := value.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(v))
		for k, child := range v {
			if sensitiveField(k, child) && child != nil && child != "" {
				cleaned[ScrubText(k)] = Redacted
			} else {
				cleaned[ScrubText(k)] = scrub(child)
			}
		}
		return cleaned
	case []any:
		for i := range v {
			v[i] = scrub(v[i])
		}
	case string:
		// Keep opaque media data usable in archived multimodal messages.
		if mediaDataURL.MatchString(v) {
			return v
		}
		// Tool outputs frequently contain JSON encoded inside a JSON string.
		if nested, err := decode(v); err == nil {
			cleaned, _ := json.Marshal(scrub(nested))
			return string(cleaned)
		}
		return ScrubText(v)
	}
	return value
}

// ScrubBody handles JSON, SSE, JSONL/WebSocket archives and plain text. It only
// operates on archived copies, never on the bytes delivered to either peer.
func ScrubBody(body string) string {
	if body == "" {
		return body
	}
	if value, err := decode(body); err == nil {
		cleaned, _ := json.Marshal(scrub(value))
		return string(cleaned)
	}
	lines := strings.Split(body, "\n")
	var text strings.Builder
	var frames int
	for i, line := range lines {
		prefix := ""
		payload := strings.TrimSpace(line)
		if strings.HasPrefix(payload, "data:") {
			prefix = "data: "
			payload = strings.TrimSpace(strings.TrimPrefix(payload, "data:"))
		}
		if value, err := decode(payload); err == nil {
			frames++
			collectText(value, &text)
			cleaned, _ := json.Marshal(scrub(value))
			lines[i] = prefix + string(cleaned)
		} else {
			lines[i] = ScrubText(line)
		}
	}
	// A key can span arbitrary delta boundaries (including escaped JSON). Drop
	// the archived stream rather than persist partially redacted fragments.
	if frames > 0 && inspect(text.String(), false) != "" {
		return "[REDACTED: credential in streamed content]"
	}
	return strings.Join(lines, "\n")
}

func collectText(value any, out *strings.Builder) {
	switch v := value.(type) {
	case map[string]any:
		// Stable field order avoids dependence on Go map iteration.
		for _, k := range []string{"choices", "delta", "message", "content", "text", "tool_calls", "function", "arguments", "partial_json", "thinking", "reasoning_content", "output", "item", "response", "candidates", "parts"} {
			if child, ok := v[k]; ok {
				collectText(child, out)
			}
		}
	case []any:
		for _, child := range v {
			collectText(child, out)
		}
	case string:
		out.WriteString(v)
	}
}

// ScrubHeader protects archived headers, including custom credential headers.
// Callers must use the result on a copy, never on the outgoing request.
func ScrubHeader(name, value string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	scheme := strings.ToLower(strings.TrimSpace(value))
	if sensitiveField(lower, value) || strings.HasSuffix(lower, "-token") || strings.HasSuffix(lower, "-secret") || strings.HasSuffix(lower, "-password") || strings.HasSuffix(lower, "-api-key") || strings.HasPrefix(scheme, "bearer ") || strings.HasPrefix(scheme, "basic ") {
		return Redacted
	}
	return ScrubText(value)
}

// ScrubURL removes URL credentials from archived snapshots. Query names supply
// context even for short values or percent-encoded credentials. Malformed URLs
// are omitted rather than risk retaining credentials in a partially parsed URL.
func ScrubURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Redacted
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Redacted
	}
	changed := false
	if u.User != nil {
		username := ScrubText(u.User.Username())
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(username, Redacted)
			changed = true
		} else if username != u.User.Username() {
			u.User = url.User(username)
			changed = true
		}
	}
	if path := ScrubText(u.Path); path != u.Path {
		u.Path = path
		u.RawPath = ""
		changed = true
	}
	if fragment := ScrubText(u.Fragment); fragment != u.Fragment {
		u.Fragment = fragment
		changed = true
	}
	for name, values := range query {
		for i, value := range values {
			cleaned := ScrubText(value)
			if sensitiveField(name, value) || strings.EqualFold(name, "key") {
				cleaned = Redacted
			}
			if cleaned != value {
				values[i] = cleaned
				changed = true
			}
		}
	}
	if !changed {
		return raw
	}
	u.RawQuery = query.Encode()
	return u.String()
}

// ScrubRequestInfo returns a scrubbed snapshot without changing transport objects.
func ScrubRequestInfo(info *domain.RequestInfo, filters ...func(string) string) *domain.RequestInfo {
	if info == nil {
		return nil
	}
	redacted := *info
	redacted.URL = ScrubURL(info.URL)
	redacted.Headers = scrubHeaders(info.Headers)
	redacted.Body = ScrubBody(info.Body)
	for _, filter := range filters {
		if filter != nil {
			redacted.URL = filter(redacted.URL)
			redacted.Body = filter(redacted.Body)
			for key, value := range redacted.Headers {
				redacted.Headers[key] = filter(value)
			}
		}
	}
	return &redacted
}

// ScrubResponseInfo returns a scrubbed snapshot without changing the client reply.
func ScrubResponseInfo(info *domain.ResponseInfo, filters ...func(string) string) *domain.ResponseInfo {
	if info == nil {
		return nil
	}
	redacted := *info
	redacted.Headers = scrubHeaders(info.Headers)
	redacted.Body = ScrubBody(info.Body)
	for _, filter := range filters {
		if filter != nil {
			redacted.Body = filter(redacted.Body)
			for key, value := range redacted.Headers {
				redacted.Headers[key] = filter(value)
			}
		}
	}
	return &redacted
}

func scrubHeaders(headers map[string]string) map[string]string {
	redacted := domain.RedactSensitiveHeaders(headers)
	for name, value := range redacted {
		redacted[name] = ScrubHeader(name, value)
	}
	return redacted
}
