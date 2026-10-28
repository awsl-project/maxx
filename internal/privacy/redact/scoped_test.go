package redact

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestScopedMappingsAndLearnedValuesStayIsolated(t *testing.T) {
	for _, shape := range []bool{false, true} {
		a, b := NewState(), NewState()
		options := Options{Secrets: true, Shape: shape}
		secret := "sk-proj-Example1234567890abcdefgh"
		masked, n := a.Mask(secret, options)
		if n != 1 || masked == secret || a.Restore(masked, false) != secret {
			t.Fatal("scoped masking roundtrip failed")
		}
		if b.Restore(masked, false) != masked {
			t.Fatal("another scope restored a private value")
		}
		if _, ok := b.Value(masked); ok {
			t.Fatal("another scope exposed a private value")
		}
		// A password learned in one scope must not influence detection elsewhere.
		password := "unlabelled-example-password-123"
		a.Mask("password="+password, options)
		if own, n := a.Mask(password, options); n != 1 || own == password {
			t.Fatal("same-scope learned value was not reused")
		}
		if other, n := b.Mask(password, options); n != 0 || other != password {
			t.Fatal("another scope inherited learned values")
		}
		preview, hits := "", 0
		preview = b.Preview(secret, Options{Secrets: true, Shape: shape, Seen: func(Hit) { hits++ }})
		if hits != 1 || b.Restore(preview, false) != preview {
			t.Fatal("preview retained reversible mappings")
		}
	}
}

func TestScopedSSEWriterRestoresSplitPlaceholders(t *testing.T) {
	state := NewState()
	secret := "sk-proj-Example1234567890abcdefgh"
	placeholder, _ := state.Mask(secret, Options{Secrets: true})
	for split := 1; split < len(placeholder); split++ {
		recorder := httptest.NewRecorder()
		writer := state.NewWriter(recorder)
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Write([]byte(`data: {"choices":[{"delta":{"content":"` + placeholder[:split] + `"}}]}` + "\n\n"))
		writer.Write([]byte(`data: {"choices":[{"delta":{"content":"` + placeholder[split:] + `"}}]}` + "\n\n"))
		writer.Finish()
		if !strings.Contains(recorder.Body.String(), secret) || strings.Contains(recorder.Body.String(), placeholder) {
			t.Fatalf("split %d not restored: %s", split, recorder.Body.String())
		}
	}
}

func TestScopedStateConcurrentRoundtrips(t *testing.T) {
	a, b := NewState(), NewState()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value := "sk-proj-Example1234567890abcdefgh"
			masked, _ := a.Mask(value, Options{Secrets: true})
			if a.Restore(masked, false) != value || b.Restore(masked, false) != masked {
				t.Error("concurrent state leaked or lost a mapping")
			}
		}()
	}
	wg.Wait()
}

func TestScopedLimitsAndOversizeRecovery(t *testing.T) {
	state := NewState()
	state.SetLimits(3, 256)
	for _, word := range []string{"Alpha123456", "Bravo123456", "Charlie123456", "Delta123456", "Echo123456"} {
		state.Mask(word, Options{Words: []string{word}})
	}
	state.mu.RLock()
	entries, size := len(state.values), state.usedBytes
	state.mu.RUnlock()
	if entries > 3 || size > 256 {
		t.Fatalf("unbounded state: %d entries %d bytes", entries, size)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("oversized mapping did not fail closed")
			}
		}()
		large := strings.Repeat("A", 300)
		state.Mask(large, Options{Words: []string{large}})
	}()
	done := make(chan bool, 1)
	go func() {
		masked, _ := state.Mask("recover123456", Options{Words: []string{"recover123456"}})
		done <- state.Restore(masked, false) == "recover123456"
	}()
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("state unusable after rejection")
		}
	case <-time.After(time.Second):
		t.Fatal("oversize rejection left a lock held")
	}
	state.Clear()
	state.mu.RLock()
	defer state.mu.RUnlock()
	if len(state.values) != 0 || state.usedBytes != 0 || len(state.known) != 0 {
		t.Fatal("clear retained originals")
	}
}
