//go:build !darwin || !cgo

package coreml

import "errors"

// Available says CoreML is here: only on a Mac (coreml_darwin.go).
const Available = false

var errNone = errors.New("no CoreML here")

func Compile(pkg, out string) error { return errNone }

type Model struct{}

func Open(path string) (*Model, error) { return nil, errNone }

func (*Model) Run(x, mask []float32, s, h, k int) ([]float32, error) { return nil, errNone }
