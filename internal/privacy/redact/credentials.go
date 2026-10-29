package redact

import (
	"regexp"
	"strings"
)

// Explicit assignments can contain short quoted values, punctuation and no
// digits. Keep this narrower than the generic SECRET name heuristic.
const literalCredentialNames = `api[_-]?key|access[_-]?token|refresh[_-]?token|auth[_-]?token|client[_-]?secret|client[_-]?token|password|passwd|secret[_-]?key|(?:aws[_-]?)?secret[_-]?access[_-]?key|aws[_-]?(?:access[_-]?key[_-]?id|session[_-]?token)|(?:db|database)[_-]?password`

var literalCodeReference = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+|\$[A-Za-z_][A-Za-z0-9_]*)$`)
var literalFunctionCall = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*\((?:.*\)(?:\.[A-Za-z_][A-Za-z0-9_]*)*|[^)]*)$`)

func literalCredentialValue(value string) bool {
	return notAVariable(value) && !strings.ContainsAny(value, "{}") && !strings.HasPrefix(value, "$(") &&
		!literalFunctionCall.MatchString(value) && !literalCodeReference.MatchString(value)
}
