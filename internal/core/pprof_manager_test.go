package core

import "testing"

func TestPprofListenAddressBindsAllInterfaces(t *testing.T) {
	if got, want := pprofListenAddress(6060), "0.0.0.0:6060"; got != want {
		t.Fatalf("pprofListenAddress(6060) = %q, want %q", got, want)
	}
}
