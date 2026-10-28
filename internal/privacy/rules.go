// Package privacy applies stateless, irreversible credential protection. Rules
// are inspired by Magpie (https://github.com/yetone/magpie); see LICENSE.magpie.
package privacy

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strings"
)

const Redacted = "[REDACTED]"

type rule struct {
	kind string
	re   *regexp.Regexp
	// group is the credential capture to replace; zero replaces the whole match.
	group  int
	accept func(string) bool
}

// Assignment names are deliberately narrower than arbitrary words like "key".
const credentialNames = `api[_-]?key|access[_-]?token|refresh[_-]?token|auth[_-]?token|client[_-]?secret|client[_-]?token|password|passwd|secret[_-]?key|(?:aws[_-]?)?secret[_-]?access[_-]?key|aws[_-]?(?:access[_-]?key[_-]?id|session[_-]?token)|(?:db|database)[_-]?password`
const credentialAssignment = `(?i)\b[A-Za-z0-9_-]*(?:` + credentialNames + `)\b["']?\s*[:=]\s*`

var credentialFieldName = regexp.MustCompile(`(?i)(?:^|[_-])(?:` + credentialNames + `)$`)

var rules = append([]rule{
	{"private_key", regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----.*?(?:-----END (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----|$)`), 0, nil},
	{"api_key", regexp.MustCompile(`\bsk-(?:proj-|ant-[A-Za-z0-9]+-)?[A-Za-z0-9_-]{20,}\b`), 0, notASlug},
	{"api_key", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,}|glpat-[A-Za-z0-9_-]{20,}|(?:AKIA|ASIA)[A-Z0-9]{16}|maxx_[A-Za-z0-9_-]{20,}|[a-z]{2,4}_sk_[A-Za-z0-9_-]{20,})\b`), 0, nil},
	{"api_key", regexp.MustCompile(`\b(?:xox[abposr]-[0-9A-Za-z-]{10,}|(?:sk|rk)_live_[0-9A-Za-z]{20,}|hf_[A-Za-z0-9]{30,}|gsk_[A-Za-z0-9]{40,}|xai-[A-Za-z0-9]{40,}|npm_[A-Za-z0-9]{36}|pypi-[A-Za-z0-9_-]{50,}|dop_v1_[a-f0-9]{64})\b`), 0, nil},
	{"api_key", regexp.MustCompile(`\b(AIza[A-Za-z0-9_-]{35}|SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43})(?:$|[^A-Za-z0-9_-])`), 1, nil},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`), 0, nil},
	{"credential", regexp.MustCompile(credentialAssignment + `"((?:\\.|[^"\\])+)(?:"|$)`), 1, nil},
	{"credential", regexp.MustCompile(credentialAssignment + `'((?:\\.|[^'\\])+)(?:'|$)`), 1, nil},
	{"credential", regexp.MustCompile(credentialAssignment + `([^\s"'` + "`" + `,;<>\\}]{8,})`), 1, credentialValue},
	{"authorization", regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9_+/.=~-]{16,})`), 1, nil},
	{"authorization", regexp.MustCompile(`(?i)\bbasic\s+([A-Za-z0-9_+/.=~-]{16,})`), 1, basicAuthValue},
	{"connection_password", regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.-]*://[^\s/:@]*:([^\s/@]+)@`), 1, nil},
}, namedRules...)

// Detect returns only a rule name, never the matched credential.
func Detect(text string) string {
	for _, r := range rules {
		for _, loc := range r.re.FindAllStringSubmatchIndex(text, -1) {
			if r.accepts(text, loc) {
				return r.kind
			}
		}
	}
	return ""
}

func ScrubText(text string) string {
	for _, r := range rules {
		var out strings.Builder
		last := 0
		for _, loc := range r.re.FindAllStringSubmatchIndex(text, -1) {
			if !r.accepts(text, loc) {
				continue
			}
			start, end := loc[2*r.group], loc[2*r.group+1]
			out.WriteString(text[last:start])
			out.WriteString(Redacted)
			last = end
		}
		if last > 0 {
			out.WriteString(text[last:])
			text = out.String()
		}
	}
	return text
}

// The same value checks apply to detection and redaction, including combined
// stream deltas. Quoted literals have separate rules without a code filter.
func (r rule) accepts(text string, loc []int) bool {
	if r.kind == "named_key" {
		return namedKeyValue(text, loc[2], loc[3])
	}
	if r.kind == "credential" && text[loc[2*r.group]:loc[2*r.group+1]] == Redacted {
		return false
	}
	if r.accept == nil {
		return true
	}
	start, end := loc[2*r.group], loc[2*r.group+1]
	return r.accept(text[start:end])
}

// Magpie excludes article slugs such as sk-hynix-third-quarter-profit from
// standalone key matches. Explicit password/API-key fields are still protected.
func notASlug(value string) bool {
	if strings.ContainsFunc(value, func(r rune) bool { return r >= 'A' && r <= 'Z' }) {
		return true
	}
	if strings.Trim(strings.TrimPrefix(value, "sk-"), "0123456789abcdef-_") == "" {
		return true
	}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '-' || r == '_' }) {
		if len(part) >= 16 {
			return true
		}
	}
	return false
}

var codeReference = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+|\$[A-Za-z_][A-Za-z0-9_]*)$`)

var functionCall = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*\((?:.*\)(?:\.[A-Za-z_][A-Za-z0-9_]*)*|[^)]*)$`)

func credentialValue(value string) bool {
	return !strings.ContainsAny(value, "{}") && !strings.HasPrefix(value, "$(") && !functionCall.MatchString(value) && !codeReference.MatchString(value)
}

func basicAuthValue(value string) bool {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(value)
	}
	return err == nil && bytes.ContainsRune(decoded, ':')
}

func sensitiveKey(key string) bool {
	if credentialFieldName.MatchString(key) {
		return true
	}
	key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	switch key {
	case "password", "passwd", "apikey", "apitoken", "secret", "privatekey", "accesstoken", "refreshtoken", "authtoken", "clientsecret", "secretkey", "authorization", "proxyauthorization", "cookie", "setcookie", "xapikey", "xgoogapikey", "awsaccesskeyid", "awssecretaccesskey", "awssessiontoken", "secretaccesskey", "dbpassword", "databasepassword", "credential", "credentials", "token":
		return true
	}
	return false
}

// Prefix/suffix naming heuristics apply to string values, not boolean flags such
// as hasPassword. Exact credential fields retain their existing behavior.
func sensitiveField(name string, value any) bool {
	if sensitiveKey(name) {
		return true
	}
	if _, ok := value.(string); !ok {
		return false
	}
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "_", ""), "-", ""))
	for _, suffix := range []string{"password", "passwd", "apikey", "token", "clientsecret", "secretkey", "secretaccesskey", "privatekey"} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}
