package executor

import (
	"testing"
)

func TestExecuteWithProviderSlotReleasesAfterPanic(t *testing.T) {
	released := false
	func() {
		defer func() { _ = recover() }()
		_ = executeWithProviderSlot(func() { released = true }, func() error {
			panic("adapter panic")
		})
	}()
	if !released {
		t.Fatal("HTTP provider slot was not released after panic")
	}
}
