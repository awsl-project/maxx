package guard

import "golang.org/x/sys/cpu"

//go:noescape
func dot4sdot(a, b *int8, k int, out *[4]int32)

func init() {
	if cpu.ARM64.HasASIMDDP {
		dot4fast = func(a, b []int8, out *[4]int32) {
			_ = b[4*len(a)-1]
			dot4sdot(&a[0], &b[0], len(a), out)
		}
	}
}

//go:noescape
func scoresT(q, kt *float32, np int, out *float32)

//go:noescape
func mix32(c, p, v *float32, n int)

func init() {
	// NEON is part of every arm64 Go supports.
	scoresFast = func(q *[32]float32, kt []float32, out []float32) {
		_ = kt[32*len(out)-1]
		scoresT(&q[0], &kt[0], len(out), &out[0])
	}
	mixFast = func(c *[32]float32, p, v []float32) {
		_ = v[32*len(p)-1]
		mix32(&c[0], &p[0], &v[0], len(p))
	}
}
