package redact

import (
	"strings"
	"sync"
	"sync/atomic"
)

// State isolates reversible mappings, learned values and caches. Maxx allocates
// one state per tenant/project; callers must never share it across projects.
// Upstream's stateless rules stay shared, while no credential value is global.
type State struct {
	capacity  int
	maxBytes  int
	usedBytes int
	mu        sync.RWMutex
	values    map[string]string
	kinds     map[string]bool
	keyOf     []byte
	keyPath   string
	known     map[string][]knownValue
	knownBits [1 << 14]uint64
	knownGen  atomic.Uint64
	scores    map[string]float64
	sentKnown struct {
		sync.Mutex
		cur, old map[[32]byte][]span
	}
	shapes    map[string]string
	scrubMemo memo
	scrubRuns atomic.Uint64
}

func NewState() *State {
	return &State{values: map[string]string{}, kinds: map[string]bool{}, known: map[string][]knownValue{}, scores: map[string]float64{}, shapes: map[string]string{}}
}

// defaultState preserves the upstream package API for its regression suite.
// The Maxx integration uses explicit State instances exclusively.
var defaultState = NewState()

func (o Options) state() *State {
	if o.State != nil {
		return o.State
	}
	return defaultState
}

func (rw *Writer) state() *State {
	if rw.scopeState != nil {
		return rw.scopeState
	}
	return defaultState
}

// SetLimits must be called before this state is used. A nonpositive bound keeps
// the upstream default; Maxx runtime sets explicit per-project memory bounds.
func (s *State) SetLimits(entries, bytes int) {
	s.capacity, s.maxBytes = entries, bytes
	if bytes > 0 {
		s.scrubMemo.limit = min(bytes, 256<<10)
	}
}

// Clear forgets original values while keeping the keyed placeholder identity.
func (s *State) Clear() {
	s.mu.Lock()
	s.values, s.shapes, s.scores, s.kinds = map[string]string{}, map[string]string{}, map[string]float64{}, map[string]bool{}
	s.usedBytes = 0
	s.reindexKnown()
	s.mu.Unlock()
	s.resetSent()
	s.scrubMemo.reset()
}

// storeValue is called with the state's write lock held.
func (s *State) storeValue(key, value string) {
	size := len(key) + len(value)

	removed := false
	for k, v := range s.values {
		if s.maxBytes <= 0 || s.usedBytes+size <= s.maxBytes {
			break
		}
		delete(s.values, k)
		delete(s.shapes, k)
		s.usedBytes -= len(k) + len(v)
		removed = true
	}
	if removed {
		s.reindexKnown()
	}
	s.values[key] = value
	s.usedBytes += size
}

// checkValueSize runs before the write lock is taken, so rejecting an oversized
// mapping cannot leave the state locked when the caller safely recovers.
func (s *State) checkValueSize(key, value string) {
	if s.maxBytes > 0 && len(key)+len(value) > s.maxBytes {
		panic("privacy mapping exceeds memory limit")
	}
}

// Restorer creates a request-local restore view. A response can restore only
// placeholders actually sent in this request, never an unrelated request's map.
func Restorer(pairs map[string]string) *State {
	state := NewState()
	for placeholder, value := range pairs {
		name, shaped := splitName(placeholder)
		if name != "" {
			_, original := splitName(value)
			placeholder, value = shaped, original
		}
		state.values[placeholder] = value
		if strings.HasPrefix(placeholder, "{{") {
			state.kinds[state.kindOf(placeholder)] = true
		} else {
			state.shapes[placeholder] = ""
		}
	}
	return state
}

// RedactText applies selected rules irreversibly without running a model,
// keeping original values, emitting hit records or honoring allow/warn actions.
func (s *State) RedactText(text string, options Options) (string, int) {
	options.State = s
	options.Model, options.Seen, options.Stored, options.Matched = nil, nil, nil, nil
	options.Allow, options.Actions, options.DefaultAction = nil, nil, ""
	options.sent = false
	return s.mask(text, options, func(_, _ string) string { return Scrubbed }, false)
}
