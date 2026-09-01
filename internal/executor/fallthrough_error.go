package executor

import (
	"fmt"
	"strings"
)

// maxSurfacedErrorMessageLen bounds the upstream error snippet we fold into the
// surfaced/logged summary so a large upstream error body can't bloat the client
// response or a log line. Enough to keep the meaningful part (e.g. a status +
// short JSON detail) while staying compact.
const maxSurfacedErrorMessageLen = 300

// attemptFailure is a compact, secret-free record of a single failed upstream
// attempt. It is only ever built from provider ids and upstream status/message
// text — never from request/response headers — so it is always safe to surface
// to the client and to logs.
type attemptFailure struct {
	providerID uint64
	status     int
	message    string
}

// summary renders a single failed attempt as "provider N: <status> <message>".
func (f attemptFailure) summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "provider %d", f.providerID)
	if f.status > 0 {
		fmt.Fprintf(&b, ": %d", f.status)
	} else {
		b.WriteString(":")
	}
	if f.message != "" {
		b.WriteString(" ")
		b.WriteString(f.message)
	}
	return b.String()
}

// newAttemptFailure builds a secret-free summary of a failed upstream attempt.
// It reads only the provider id and the error's own message/status — the error
// message is an upstream status/body snippet, which already excludes auth
// headers (those are redacted before persistence elsewhere). The message is
// truncated so no oversized upstream body leaks into the surfaced error or log.
func newAttemptFailure(providerID uint64, err error) attemptFailure {
	f := attemptFailure{providerID: providerID}
	if err == nil {
		return f
	}
	if proxyErr, ok := asProxyError(err); ok {
		if proxyErr.HTTPStatusCode >= 400 && proxyErr.HTTPStatusCode < 600 {
			f.status = proxyErr.HTTPStatusCode
		}
	}
	f.message = truncateForSurface(strings.TrimSpace(err.Error()))
	return f
}

func truncateForSurface(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxSurfacedErrorMessageLen {
		return s
	}
	return s[:maxSurfacedErrorMessageLen] + "…"
}

// attemptTrailSummary renders the whole failed-attempt chain as a single
// "provider A: ... -> provider B: ..." string for one-line WARN logging.
func attemptTrailSummary(trail []attemptFailure) string {
	parts := make([]string, 0, len(trail))
	for _, f := range trail {
		parts = append(parts, f.summary())
	}
	return strings.Join(parts, " -> ")
}

// firstInformativeFailure returns the earliest recorded attempt failure that
// carries a distinctive upstream signal (an HTTP status or a non-empty
// message). The first provider tried is usually the one with the real cause
// (e.g. "fal returned status 403: User is locked"), which a later catch-all
// provider's unrelated error (e.g. "404 no model found") would otherwise mask.
func firstInformativeFailure(trail []attemptFailure) (attemptFailure, bool) {
	for _, f := range trail {
		if f.status > 0 || f.message != "" {
			return f, true
		}
	}
	return attemptFailure{}, false
}

// trailInvolvesMultipleProviders reports whether the recorded failures span
// more than one distinct provider — i.e. an actual cross-provider fallthrough
// happened, as opposed to same-provider retries. Only then is surfacing the
// first provider's error useful (it would otherwise be identical to the final
// error).
func trailInvolvesMultipleProviders(trail []attemptFailure) bool {
	var first uint64
	seen := false
	for _, f := range trail {
		if !seen {
			first = f.providerID
			seen = true
			continue
		}
		if f.providerID != first {
			return true
		}
	}
	return false
}

// augmentFallthroughError, when a request fails after failing over across more
// than one provider, folds a short summary of the first informative upstream
// error into the final error's message. This makes the true first-provider
// cause observable to the client instead of only the last (often catch-all)
// provider's unrelated error. Routing/failover behavior is unchanged — this
// only enriches the surfaced message. It returns the (possibly mutated) error,
// the first informative failure, and a bool indicating whether augmentation was
// applied.
func augmentFallthroughError(finalErr error, trail []attemptFailure) (error, attemptFailure, bool) {
	if finalErr == nil {
		return finalErr, attemptFailure{}, false
	}
	if !trailInvolvesMultipleProviders(trail) {
		return finalErr, attemptFailure{}, false
	}
	first, ok := firstInformativeFailure(trail)
	if !ok {
		return finalErr, attemptFailure{}, false
	}

	proxyErr, ok := asProxyError(finalErr)
	if !ok {
		return finalErr, first, false
	}
	// Don't restate the same thing if the final error already carries the first
	// provider's error (nothing was masked).
	if first.message != "" && strings.Contains(proxyErr.Error(), first.message) {
		return finalErr, attemptFailure{}, false
	}

	note := "first upstream " + first.summary()
	if proxyErr.Message != "" {
		proxyErr.Message = proxyErr.Message + " [" + note + "]"
	} else {
		proxyErr.Message = note
	}
	return proxyErr, first, true
}
