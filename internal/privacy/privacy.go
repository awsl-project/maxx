// Package privacy applies stateless, irreversible credential protection. Rules
// are inspired by Magpie (https://github.com/yetone/magpie); see LICENSE.magpie.
package privacy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"regexp"
	"strings"
)

const Redacted = "[REDACTED]"

type rule struct {
	kind string
	re   *regexp.Regexp
	// group is the credential capture to replace; zero replaces the whole match.
	group int
}

var rules = []rule{
	{"private_key", regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----.*?(?:-----END (?:[A-Z0-9]+ )*PRIVATE KEY-----|$)`), 0},
	{"api_key", regexp.MustCompile(`\b(?:sk-(?:proj-|ant-[A-Za-z0-9]+-)?[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,}|AIza[A-Za-z0-9_-]{35}|(?:AKIA|ASIA)[A-Z0-9]{16}|maxx_[A-Za-z0-9_-]{20,})\b`), 0},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`), 0},
	{"credential", regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|auth[_-]?token|client[_-]?secret|password|passwd|secret[_-]?key)\b["']?\s*[:=]\s*["']?([^\s"'` + "`" + `,;<>\\}]{8,})`), 1},
	{"authorization", regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+([A-Za-z0-9_+/.=~-]{16,})`), 1},
	{"connection_password", regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.-]*://[^\s/:@]+:([^\s/@]+)@`), 1},
}

// Detect returns only a rule name, never the matched credential.
func Detect(text string) string {
	for _, r := range rules {
		if r.re.MatchString(text) {
			return r.kind
		}
	}
	return ""
}

func ScrubText(text string) string {
	for _, r := range rules {
		if r.group == 0 {
			text = r.re.ReplaceAllString(text, Redacted)
			continue
		}
		text = r.re.ReplaceAllStringFunc(text, func(match string) string {
			loc := r.re.FindStringSubmatchIndex(match)
			return match[:loc[2*r.group]] + Redacted + match[loc[2*r.group+1]:]
		})
	}
	return text
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
	switch key {
	case "password", "passwd", "apikey", "apitoken", "secret", "privatekey", "accesstoken", "refreshtoken", "authtoken", "clientsecret", "secretkey", "authorization", "proxyauthorization", "cookie", "setcookie", "xapikey", "xgoogapikey":
		return true
	}
	return false
}

func decode(body string) (any, error) {
	d := json.NewDecoder(strings.NewReader(body))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("expected one JSON value")
	}
	return value, nil
}

func scrub(value any) any {
	switch v := value.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(v))
		for k, child := range v {
			if sensitiveKey(k) && child != nil && child != "" {
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

// InspectRequest checks message content and tool results, not authentication
// headers, model names, tool schemas or opaque protocol identifiers. Inspection
// does not modify the request. Unparseable/unsupported input fails closed when
// project protection is enabled. Binary file attachments are not inspected.
func InspectRequest(body []byte, contentType string) (string, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return "", errors.New("invalid content type")
	}
	if mediaType == "multipart/form-data" {
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				return "", nil
			}
			if err != nil {
				return "", errors.New("invalid multipart body")
			}
			if part.FileName() != "" {
				continue
			}
			text, err := io.ReadAll(part)
			if err != nil {
				return "", errors.New("invalid multipart field")
			}
			if kind := Detect(string(text)); kind != "" {
				return kind, nil
			}
		}
	}
	if mediaType != "" && mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return "", errors.New("unsupported request body")
	}
	value, err := decode(string(body))
	if err != nil {
		return "", errors.New("invalid JSON body")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return "", errors.New("expected JSON object")
	}
	for _, key := range []string{"messages", "system", "input", "instructions", "contents", "systemInstruction", "system_instruction", "prompt"} {
		if kind := inspect(object[key], true); kind != "" {
			return kind, nil
		}
	}
	return "", nil
}

func inspect(value any, protocol bool) string {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			if sensitiveKey(k) && child != nil && child != "" {
				return "credential_field"
			}
			if protocol {
				switch k {
				case "id", "call_id", "tool_call_id", "type", "role", "name", "signature", "thoughtSignature", "encrypted_content", "image_url", "image", "audio", "inlineData", "inline_data", "file_data", "file_id", "file_url", "file_data_url", "data":
					continue
				}
			}
			if kind := inspect(child, protocol); kind != "" {
				return kind
			}
		}
	case []any:
		for _, child := range v {
			if kind := inspect(child, protocol); kind != "" {
				return kind
			}
		}
	case string:
		if kind := Detect(v); kind != "" {
			return kind
		}
		if nested, err := decode(v); err == nil {
			return inspect(nested, false)
		}
	}
	return ""
}
