// Package error_fixer provides a pluggable framework for detecting and fixing
// upstream API errors that can be resolved by modifying the request and retrying.
//
// Each fixer is a separate file that registers itself via init().
// To add a new error case, create a new file and register an ErrorFixer.
package error_fixer

import (
	"net/http"
	"sort"

	"github.com/awsl-project/maxx/internal/domain"
)

// ErrorFixer describes one class of upstream error that can be fixed by
// modifying the request and retrying.
type ErrorFixer interface {
	// Name returns a short identifier for logging (e.g. "cache_control").
	Name() string

	// Priority returns the execution order. Lower value = higher priority.
	// Broad fixers (e.g. bedrock) should use low values (0-99) so they run
	// first and handle everything. Narrow fixers use higher values (100+).
	Priority() int

	// MatchResponse checks whether the upstream error response matches this fixer.
	// resp may be nil for SSE errors where no distinct HTTP error response exists.
	// body is the raw error content (HTTP response body or SSE error event data),
	// passed separately because resp.Body is already consumed.
	MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool

	// FixRequest modifies a clone of the original request to work around the error.
	// req is safe to modify in place (caller passes a clone).
	// body is the raw request body bytes.
	// Returns the (possibly modified) request and new body.
	FixRequest(req *http.Request, body []byte) (*http.Request, []byte)
}

var registry []ErrorFixer

// Register adds a fixer to the global registry. Called from init().
func Register(f ErrorFixer) {
	registry = append(registry, f)
	sort.Slice(registry, func(i, j int) bool {
		return registry[i].Priority() < registry[j].Priority()
	})
}

// FindFixers returns all fixers that match the error response, sorted by priority.
func FindFixers(resp *http.Response, body []byte, clientType domain.ClientType) []ErrorFixer {
	var matched []ErrorFixer
	for _, f := range registry {
		if f.MatchResponse(resp, body, clientType) {
			matched = append(matched, f)
		}
	}
	return matched
}
