package redact

// ModelKinds is modelKinds, for the evaluation in package redact_test,
// which reads the real model through internal/pii (eval_test.go).
var ModelKinds = modelKinds

// Forget lets go of every value masked so far, so that what the
// evaluation scores is what its texts make masked, not what the tests
// before it masked and left known.
func Forget() {
	defaultState.mu.Lock()
	defaultState.values = map[string]string{}
	defaultState.shapes = map[string]string{}
	defaultState.scores = map[string]float64{}
	reindexKnown()
	defaultState.mu.Unlock()
	resetSent()
}
