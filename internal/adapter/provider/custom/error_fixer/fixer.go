// Package error_fixer provides a pluggable framework for detecting and fixing
// upstream API errors that can be resolved by modifying the request and retrying.
//
// Each fixer is a separate file that registers itself via init().
// To add a new error case, create a new file and register an ErrorFixer.
//
// File naming convention: files are prefixed with the fixer's priority value
// (e.g. 000_ for priority 0, 100_ for priority 100) so that related fixers
// are visually grouped and the priority is immediately obvious.
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
	//
	// Priority is exclusive: if any fixer at a given priority level matches,
	// fixers at higher (numerically larger) priority levels are skipped.
	// This prevents redundant work when a broad fixer already covers
	// everything the narrow fixers would do.
	Priority() int

	// MatchResponse checks whether the upstream error response matches this fixer.
	// resp may be nil for SSE errors where no distinct HTTP error response exists.
	// body is the raw error content (HTTP response body or SSE error event data),
	// passed separately because resp.Body is already consumed.
	MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool

	// FixRequest modifies a clone of the original request to work around the error.
	// The caller passes a clone via http.Request.Clone(), so req is safe to mutate.
	// Fixers must not hold references to req beyond this call.
	// body is the raw request body bytes; fixers must not mutate the input slice.
	// Returns the (possibly modified) request and a new body slice.
	FixRequest(req *http.Request, body []byte) (*http.Request, []byte)
}

var registry []ErrorFixer

// Register adds a fixer to the global registry.
// Must only be called from init(); not safe for concurrent use.
func Register(f ErrorFixer) {
	registry = append(registry, f)
	sort.Slice(registry, func(i, j int) bool {
		return registry[i].Priority() < registry[j].Priority()
	})
}

// FindFixers returns matching fixers in priority order.
// Priority is exclusive: if any fixer matches at priority level N,
// all fixers with priority > N are skipped.
func FindFixers(resp *http.Response, body []byte, clientType domain.ClientType) []ErrorFixer {
	var matched []ErrorFixer
	matchedPriority := -1

	for _, f := range registry {
		// Skip if we already have matches at a lower (better) priority
		if matchedPriority >= 0 && f.Priority() > matchedPriority {
			break // registry is sorted, no need to continue
		}
		if f.MatchResponse(resp, body, clientType) {
			matched = append(matched, f)
			matchedPriority = f.Priority()
		}
	}
	return matched
}
