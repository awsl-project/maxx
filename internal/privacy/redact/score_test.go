package redact

import (
	"strings"
	"testing"
)

// A value only the model found is masked again as known only while the
// user's slider is at or below the score it was found at (_kaichen on X,
// 10-28: "即使把匹配拉到 0.95，也是一样行为"). One a rule found is known
// whatever the slider says.
func TestKnownModelValuesFollowTheSlider(t *testing.T) {
	Forget()
	defer Forget()
	const name = "Quentin Marlowe"
	scored := func(min float64) Model {
		return func(texts []string) [][]Found {
			out := make([][]Found, len(texts))
			for i, s := range texts {
				if j := strings.Index(s, "met "+name); j >= 0 && 0.6 >= min {
					out[i] = []Found{{Start: j + 4, End: j + 4 + len(name), Label: "private_person", Score: 0.6}}
				}
			}
			return out
		}
	}
	o := func(min float64) Options {
		return Options{Secrets: true, Personal: true, Model: scored(min), MinScore: min}
	}
	if out, _ := Mask("I met "+name+" today", o(0.5)); strings.Contains(out, name) {
		t.Fatalf("not found at 0.5: %q", out)
	}
	later := "a note from " + name + " came"
	if out, _ := Mask(later, o(0.5)); strings.Contains(out, name) {
		t.Errorf("known at 0.5, under the slider's 0.5: %q", out)
	}
	if out, _ := Mask(later+" again", o(0.95)); !strings.Contains(out, name) {
		t.Errorf("the model's 0.6 masked as known at 0.95: %q", out)
	}
	// a rule's value: known at any slider
	const mail = "quentin.marlowe@acme.io"
	Mask("write to "+mail, o(0.95))
	defaultState.mu.RLock()
	sc := defaultState.scores[mail]
	defaultState.mu.RUnlock()
	if sc != ruleFound {
		t.Errorf("a rule's value at %v, not known at any slider", sc)
	}
}
