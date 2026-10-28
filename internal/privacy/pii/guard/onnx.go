package guard

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

// An ONNX file is a protobuf message; only its graph's initializers (the
// weights) are read here, by the fields' numbers in onnx.proto. The
// network itself is bert.go's, written for this one model: its file is
// pinned by its hash, so its weights' names and shapes are known.

// tensor is an initializer: its shape, its element type (onnx.proto's
// TensorProto.DataType) and its bytes, little-endian.
type tensor struct {
	dims  []int64
	dtype int
	raw   []byte
}

const (
	tFloat = 1
	tUint8 = 2
	tInt8  = 3
	tInt32 = 6
	tInt64 = 7
)

// readONNX reads the initializers of the model at path, the ones kept in
// an external data file (beside it) too.
func readONNX(path string) (map[string]*tensor, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var graph []byte
	err = fields(b, func(num int, wire int, v uint64, data []byte) error {
		if num == 7 && wire == 2 { // ModelProto.graph
			graph = data
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if graph == nil {
		return nil, errors.New("no graph")
	}
	out := map[string]*tensor{}
	ext := map[string][]byte{}
	err = fields(graph, func(num int, wire int, v uint64, data []byte) error {
		if num != 5 || wire != 2 { // GraphProto.initializer
			return nil
		}
		name, t, err := readTensor(data, filepath.Dir(path), ext)
		if err != nil {
			return fmt.Errorf("initializer %q: %w", name, err)
		}
		out[name] = t
		return nil
	})
	return out, err
}

func readTensor(b []byte, dir string, ext map[string][]byte) (string, *tensor, error) {
	t := &tensor{}
	var name, location string
	var offset, length int64 = 0, -1
	var floats []float32
	var ints []int64
	external := false
	err := fields(b, func(num int, wire int, v uint64, data []byte) error {
		switch num {
		case 1: // dims
			if wire == 2 {
				return varints(data, func(x uint64) { t.dims = append(t.dims, int64(x)) })
			}
			t.dims = append(t.dims, int64(v))
		case 2:
			t.dtype = int(v)
		case 4: // float_data
			if wire == 2 {
				for i := 0; i+4 <= len(data); i += 4 {
					floats = append(floats, math.Float32frombits(binary.LittleEndian.Uint32(data[i:])))
				}
			} else if wire == 5 {
				floats = append(floats, math.Float32frombits(uint32(v)))
			}
		case 5, 7: // int32_data, int64_data
			if wire == 2 {
				return varints(data, func(x uint64) { ints = append(ints, int64(x)) })
			}
			ints = append(ints, int64(v))
		case 8:
			name = string(data)
		case 9:
			t.raw = data
		case 13: // external_data: key, value
			var k, val string
			fields(data, func(n int, _ int, _ uint64, d []byte) error {
				if n == 1 {
					k = string(d)
				} else if n == 2 {
					val = string(d)
				}
				return nil
			})
			switch k {
			case "location":
				location = val
			case "offset":
				offset, _ = strconv.ParseInt(val, 10, 64)
			case "length":
				length, _ = strconv.ParseInt(val, 10, 64)
			}
		case 14:
			external = v == 1
		}
		return nil
	})
	if err != nil {
		return name, nil, err
	}
	n := int64(1)
	for _, d := range t.dims {
		n *= d
	}
	size := map[int]int64{tFloat: 4, tUint8: 1, tInt8: 1, tInt32: 4, tInt64: 8}[t.dtype]
	if size == 0 {
		return name, nil, fmt.Errorf("element type %d", t.dtype)
	}
	switch {
	case external:
		if location != filepath.Base(location) {
			return name, nil, fmt.Errorf("external data at %q", location)
		}
		data, ok := ext[location]
		if !ok {
			data, err = os.ReadFile(filepath.Join(dir, location))
			if err != nil {
				return name, nil, err
			}
			ext[location] = data
		}
		if length < 0 {
			length = n * size
		}
		if offset < 0 || offset+length > int64(len(data)) {
			return name, nil, errors.New("external data out of range")
		}
		t.raw = data[offset : offset+length]
	case t.raw != nil:
	case floats != nil:
		t.raw = make([]byte, 4*len(floats))
		for i, f := range floats {
			binary.LittleEndian.PutUint32(t.raw[4*i:], math.Float32bits(f))
		}
	case ints != nil:
		t.raw = make([]byte, int(size)*len(ints))
		for i, x := range ints {
			switch size {
			case 1:
				t.raw[i] = byte(x)
			case 4:
				binary.LittleEndian.PutUint32(t.raw[4*i:], uint32(x))
			case 8:
				binary.LittleEndian.PutUint64(t.raw[8*i:], uint64(x))
			}
		}
	}
	if int64(len(t.raw)) != n*size {
		return name, nil, fmt.Errorf("%d bytes for %d elements", len(t.raw), n)
	}
	return name, t, nil
}

// fields calls f with each field of the protobuf message b: its number,
// wire type, and its value (a varint's or fixed's) or bytes.
func fields(b []byte, f func(num, wire int, v uint64, data []byte) error) error {
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return errors.New("bad protobuf key")
		}
		b = b[n:]
		num, wire := int(key>>3), int(key&7)
		var v uint64
		var data []byte
		switch wire {
		case 0:
			v, n = binary.Uvarint(b)
			if n <= 0 {
				return errors.New("bad varint")
			}
			b = b[n:]
		case 1:
			if len(b) < 8 {
				return errors.New("short fixed64")
			}
			v, b = binary.LittleEndian.Uint64(b), b[8:]
		case 5:
			if len(b) < 4 {
				return errors.New("short fixed32")
			}
			v, b = uint64(binary.LittleEndian.Uint32(b)), b[4:]
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || l > uint64(len(b)-n) {
				return errors.New("bad length")
			}
			data, b = b[n:n+int(l)], b[n+int(l):]
		default:
			return fmt.Errorf("wire type %d", wire)
		}
		if err := f(num, wire, v, data); err != nil {
			return err
		}
	}
	return nil
}

func varints(b []byte, f func(uint64)) error {
	for len(b) > 0 {
		x, n := binary.Uvarint(b)
		if n <= 0 {
			return errors.New("bad packed varint")
		}
		f(x)
		b = b[n:]
	}
	return nil
}

func (t *tensor) floats() []float32 {
	out := make([]float32, len(t.raw)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(t.raw[4*i:]))
	}
	return out
}

func (t *tensor) int64s() []int64 {
	out := make([]int64, len(t.raw)/8)
	for i := range out {
		out[i] = int64(binary.LittleEndian.Uint64(t.raw[8*i:]))
	}
	return out
}
