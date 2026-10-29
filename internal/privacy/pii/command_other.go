//go:build !windows

package pii

import "os/exec"

func modelCommand(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }
