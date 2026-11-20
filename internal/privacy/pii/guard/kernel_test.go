package guard

import (
	"math"
	"math/rand"
	"testing"
)

// The kernels in kernel_*.s give what the Go beside them gives: int8
// exactly, float32 to rounding. Each case is run with the kernel and with
// it switched off.
func TestKernelsMatchGo(t *testing.T) {
	if dot4fast == nil || scoresFast == nil || mixFast == nil {
		t.Log("no kernels on this CPU; the Go runs both sides")
	}
	r := rand.New(rand.NewSource(1))
	for _, k := range []int{16, 32, 384, 1536, 40} {
		x := make([]int8, k)
		w := make([]int8, 4*k)
		for i := range x {
			x[i] = int8(r.Intn(256) - 128)
		}
		for i := range w {
			w[i] = int8(r.Intn(256) - 128)
		}
		x[0], w[0] = -128, -128 // the extremes
		var got, want [4]int32
		dot4(x, w, &got)
		f := dot4fast
		dot4fast = nil
		dot4(x, w, &want)
		dot4fast = f
		if got != want {
			t.Fatalf("dot4 over %d: %v, Go %v", k, got, want)
		}
	}
	for _, n := range []int{1, 7, 16, 37, 512} {
		np := (n + 15) &^ 15
		var q [32]float32
		for i := range q {
			q[i] = r.Float32()*2 - 1
		}
		kt := make([]float32, 32*np)
		for i := range kt {
			kt[i] = r.Float32()*2 - 1
		}
		got, want := make([]float32, np), make([]float32, np)
		scores(&q, kt, got)
		f := scoresFast
		scoresFast = nil
		scores(&q, kt, want)
		scoresFast = f
		for u := range got {
			if math.Abs(float64(got[u]-want[u])) > 1e-4 {
				t.Fatalf("scores over %d keys, [%d]: %v, Go %v", n, u, got[u], want[u])
			}
		}
		p := make([]float32, n)
		for i := range p {
			p[i] = r.Float32()
		}
		v := make([]float32, 32*n)
		for i := range v {
			v[i] = r.Float32()*2 - 1
		}
		var c1, c2 [32]float32
		c1[31], c2[31] = 1, 1 // added to, not overwritten
		mix(&c1, p, v)
		m := mixFast
		mixFast = nil
		mix(&c2, p, v)
		mixFast = m
		for i := range c1 {
			if math.Abs(float64(c1[i]-c2[i])) > 1e-3 {
				t.Fatalf("mix over %d, [%d]: %v, Go %v", n, i, c1[i], c2[i])
			}
		}
	}
}

func TestExpfAndGelu(t *testing.T) {
	for x := float32(-87); x <= 0; x += 0.0013 {
		w := math.Exp(float64(x))
		if e := math.Abs(float64(expf(x))-w) / w; e > 3e-7 {
			t.Fatalf("expf(%v) = %v, want %v", x, expf(x), w)
		}
	}
	if expf(-100) != 0 {
		t.Fatal("expf(-100) not 0")
	}
	for x := float32(-10); x <= 10; x += 0.0007 {
		w := 0.5 * float64(x) * (1 + math.Erf(float64(x)/math.Sqrt2))
		if e := math.Abs(float64(gelu(x)) - w); e > 2e-6 {
			t.Fatalf("gelu(%v) = %v, want %v", x, gelu(x), w)
		}
	}
}
