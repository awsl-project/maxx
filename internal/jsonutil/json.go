package jsonutil

import (
	"runtime"
	"unsafe"

	"github.com/bytedance/sonic"
)

// fast uses Sonic's fastest configuration for performance-critical internal
// JSON paths. Callers must not rely on encoding/json's HTML escaping behavior.
var fast = sonic.ConfigFastest

func Marshal(value any) ([]byte, error) {
	return fast.Marshal(value)
}

// Unmarshal decodes immutable JSON bytes without Sonic's default whole-buffer
// []byte-to-string copy. Decoded string values may reference data, so callers
// must not modify the input while the decoded value is still in use.
func Unmarshal(data []byte, value any) error {
	if len(data) == 0 {
		return fast.UnmarshalFromString("", value)
	}
	source := unsafe.String(unsafe.SliceData(data), len(data))
	err := fast.UnmarshalFromString(source, value)
	runtime.KeepAlive(data)
	return err
}

func UnmarshalString(data string, value any) error {
	return fast.UnmarshalFromString(data, value)
}
