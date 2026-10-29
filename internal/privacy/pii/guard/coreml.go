package guard

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/awsl-project/maxx/internal/privacy/pii/guard/coreml"
)

// On a Mac the encoder runs on CoreML, which puts it on the Neural Engine
// or the GPU: a window of 512 tokens takes about 12 ms there, where
// bert.go takes 400 (an M4 Pro). The program (coreml.mlmodel, made by
// coreml_gen.py) is the same network from the same int8 weights,
// dequantized to float16 and written here from the model downloaded, so
// nothing else is fetched; it is compiled once (about 5 s) and kept
// beside the model. Held to the publisher's float32 model, its
// probabilities are within .07 where bert.go's are within .13, with the
// same labels. When it can't be made or fails, bert.go reads; a weight
// that isn't the one the program was made from (another revision) is
// such a failure, and coreml_gen.py is run again for it.

//go:embed coreml.mlmodel
var coremlProgram []byte

//go:embed coreml.json
var coremlLayout []byte

// coremlMin is the fewest tokens a window goes to CoreML with: under it
// bert.go is as quick as a padded window (8 tokens 9 ms, 16 tokens 17).
const coremlMin = 16

// openCoreML opens the program for nt, compiling it into dir/coreml
// first if it isn't there.
func openCoreML(dir string, nt *net) (*coreml.Model, error) {
	if !coreml.Available {
		return nil, errors.New("no CoreML here")
	}
	sum := sha256.New()
	sum.Write(coremlProgram)
	sum.Write(coremlLayout)
	path := filepath.Join(dir, "coreml", hex.EncodeToString(sum.Sum(nil))[:12]+".mlmodelc")
	if _, err := os.Stat(filepath.Join(path, "model.mil")); err != nil {
		if err := compileCoreML(path, nt); err != nil {
			return nil, err
		}
	}
	return coreml.Open(path)
}

// compileCoreML writes the package, its weights from nt, and compiles it
// to path; the folders of other programs are removed once it is there.
func compileCoreML(path string, nt *net) error {
	w, err := coremlWeights(nt)
	if err != nil {
		return err
	}
	root := filepath.Dir(path)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(root, "pkg-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	pkg := filepath.Join(tmp, "m.mlpackage")
	data := filepath.Join(pkg, "Data", "com.apple.CoreML")
	if err := os.MkdirAll(filepath.Join(data, "weights"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pkg, "Manifest.json"), []byte(coremlManifest), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(data, "model.mlmodel"), coremlProgram, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(data, "weights", "weight.bin"), w, 0o644); err != nil {
		return err
	}
	if err := coreml.Compile(pkg, path); err != nil {
		return err
	}
	es, _ := os.ReadDir(root)
	for _, e := range es {
		if n := e.Name(); n != filepath.Base(path) && n != filepath.Base(tmp) {
			os.RemoveAll(filepath.Join(root, n))
		}
	}
	return nil
}

const coremlManifest = `{
    "fileFormatVersion": "1.0.0",
    "itemInfoEntries": {
        "3855EAB2-EF6A-4533-A292-4BC2F7AA31FD": {
            "author": "com.apple.CoreML",
            "description": "CoreML Model Weights",
            "name": "weights",
            "path": "com.apple.CoreML/weights"
        },
        "F8FABAE7-63BE-4D77-ABB4-5EFDA56D0923": {
            "author": "com.apple.CoreML",
            "description": "CoreML Model Specification",
            "name": "model.mlmodel",
            "path": "com.apple.CoreML/model.mlmodel"
        }
    },
    "rootModelIdentifier": "F8FABAE7-63BE-4D77-ABB4-5EFDA56D0923"
}
`

// coremlWeights is the program's weight.bin from nt's weights, as
// coremltools writes it: a 64-byte header (the count, version 2), then
// each weight at the offset coreml.json gives, a 64-byte header (0xDEADBEEF,
// type 1 for float16, its size, where its data starts) and its float16s.
func coremlWeights(nt *net) ([]byte, error) {
	var lay struct {
		Size   int
		Sha256 string
		Blobs  [][2]any
	}
	if err := json.Unmarshal(coremlLayout, &lay); err != nil {
		return nil, err
	}
	src := map[string][]float32{}
	deq := func(m *qmat) []float32 {
		out := make([]float32, len(m.w))
		for c := 0; c < m.n; c++ {
			for r := 0; r < m.k; r++ {
				out[c*m.k+r] = float32(int32(m.w[c*m.k+r])-m.zp[c]) * m.scale[c]
			}
		}
		return out
	}
	for i := range nt.layers {
		l := &nt.layers[i]
		for k, v := range map[string][]float32{
			"qkv_weight": deq(&l.qkv), "qkv_bias": l.qkvBias,
			"ao_weight": deq(&l.attnOut), "ao_bias": l.attnBias,
			"ln1_gamma": l.ln1w, "ln1_beta": l.ln1b,
			"up_weight": deq(&l.up), "up_bias": l.upBias, "smooth_y": l.smooth,
			"dn_weight": deq(&l.down), "dn_bias": l.downBias,
			"ln2_gamma": l.ln2w, "ln2_beta": l.ln2b,
		} {
			src[fmt.Sprintf("l%d_%s", i, k)] = v
		}
	}
	src["logits_weight"], src["logits_bias"] = deq(&nt.cls), nt.clsBias
	out := make([]byte, lay.Size)
	le := binary.LittleEndian
	le.PutUint32(out, uint32(len(lay.Blobs)))
	le.PutUint32(out[4:], 2)
	for _, b := range lay.Blobs {
		key, _ := b[0].(string)
		off, _ := b[1].(float64)
		v, ok := src[key]
		at := int(off)
		if !ok || at+64+2*len(v) > len(out) {
			return nil, fmt.Errorf("the CoreML program's %s isn't the model's", key)
		}
		le.PutUint32(out[at:], 0xDEADBEEF)
		le.PutUint32(out[at+4:], 1)
		le.PutUint64(out[at+8:], uint64(2*len(v)))
		le.PutUint64(out[at+16:], uint64(at+64))
		for i, f := range v {
			le.PutUint16(out[at+64+2*i:], half(f))
		}
		delete(src, key)
	}
	if len(src) != 0 {
		return nil, fmt.Errorf("the CoreML program has %d weights fewer than the model", len(src))
	}
	if s := sha256.Sum256(out); hex.EncodeToString(s[:]) != lay.Sha256 {
		return nil, errors.New("the CoreML weights aren't the ones the program was made with")
	}
	return out, nil
}

// half is f in float16, rounded to the nearest even as numpy does.
func half(f float32) uint16 {
	b := math.Float32bits(f)
	sign := uint16(b>>16) & 0x8000
	man := b & 0x7fffff
	if b&0x7fffffff >= 0x7f800000 {
		if man != 0 {
			return sign | 0x7e00
		}
		return sign | 0x7c00
	}
	exp := int32(b>>23&0xff) - 127 + 15
	if exp >= 31 {
		return sign | 0x7c00
	}
	if exp <= 0 { // under float16's normals
		if exp < -10 {
			return sign
		}
		man |= 0x800000
		shift := uint32(14 - exp)
		h, rem, mid := man>>shift, man&(1<<shift-1), uint32(1)<<(shift-1)
		if rem > mid || rem == mid && h&1 == 1 {
			h++
		}
		return sign | uint16(h)
	}
	h, rem := uint32(exp)<<10|man>>13, man&0x1fff
	if rem > 0x1000 || rem == 0x1000 && h&1 == 1 {
		h++ // a carry into the exponent is still right
	}
	return sign | uint16(h)
}

// coremlLogits are nt.logits(ids), from m.
func coremlLogits(m *coreml.Model, nt *net, ids []int) ([]float32, error) {
	n := len(ids)
	if n > maxPos {
		return nil, errors.New("more tokens than a window")
	}
	mask := make([]float32, maxPos)
	for t := n; t < maxPos; t++ {
		mask[t] = -1e4
	}
	out, err := m.Run(nt.embed(ids, maxPos), mask, maxPos, hidden, nt.labels)
	if err != nil {
		return nil, err
	}
	return out[:n*nt.labels], nil
}
