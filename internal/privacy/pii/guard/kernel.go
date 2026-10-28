package guard

import "math"

// dot4fast, where the CPU has the instructions for it (kernel_*.go), sets
// out[j] to the dot product of a with b[j*len(a):(j+1)*len(a)], for
// len(a) a multiple of 32.
var dot4fast func(a, b []int8, out *[4]int32)

// dot4 is dot4fast, or the same in Go.
func dot4(a, b []int8, out *[4]int32) {
	k := len(a)
	if dot4fast != nil && k%32 == 0 && k > 0 {
		dot4fast(a, b, out)
		return
	}
	for j := range out {
		out[j] = dot(a, b[j*k:j*k+k])
	}
}

func dot(a, b []int8) int32 {
	b = b[:len(a)]
	var s0, s1, s2, s3 int32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		s0 += int32(a[i]) * int32(b[i])
		s1 += int32(a[i+1]) * int32(b[i+1])
		s2 += int32(a[i+2]) * int32(b[i+2])
		s3 += int32(a[i+3]) * int32(b[i+3])
	}
	for ; i < len(a); i++ {
		s0 += int32(a[i]) * int32(b[i])
	}
	return s0 + s1 + s2 + s3
}

// dot32 is a·b for one attention head's rows.
func dot32(a, b *[32]float32) float32 {
	var s0, s1, s2, s3 float32
	for i := 0; i < 32; i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	return (s0 + s1) + (s2 + s3)
}

// axpy32 adds w·v to c.
func axpy32(c *[32]float32, w float32, v *[32]float32) {
	for i := range c {
		c[i] += w * v[i]
	}
}

// scoresFast and mixFast, where the CPU has the instructions for them,
// do what scores and mix do.
var (
	scoresFast func(q *[32]float32, kt []float32, out []float32)
	mixFast    func(c *[32]float32, p, v []float32)
)

// scores sets out[u] to Σ q[i]·kt[i*len(out)+u]: q against each key, with
// the keys transposed, len(out) of them (a multiple of 16).
func scores(q *[32]float32, kt []float32, out []float32) {
	if scoresFast != nil && len(out) > 0 {
		scoresFast(q, kt, out)
		return
	}
	np := len(out)
	clear(out)
	for i, w := range q {
		row := kt[i*np : i*np+np]
		for u, x := range row {
			out[u] += w * x
		}
	}
}

// mix adds Σ p[u]·v[u] to c, for v's rows of 32.
func mix(c *[32]float32, p, v []float32) {
	if mixFast != nil && len(p) > 0 {
		mixFast(c, p, v)
		return
	}
	for u, w := range p {
		axpy32(c, w, (*[32]float32)(v[u*32:]))
	}
}

// expf is e^x in float32, to about an ulp (Cephes' expf), for the
// softmax's x ≤ 0 and the GELU's erf.
func expf(x float32) float32 {
	if x < -87.3 {
		return 0
	}
	if x > 88.7 {
		return float32(math.Inf(1))
	}
	n := float32(math.Floor(float64(x*1.44269504088896341 + 0.5)))
	x -= n * 0.693359375
	x -= n * -2.12194440e-4
	z := x * x
	y := ((((1.9875691500e-4*x+1.3981999507e-3)*x+8.3334519073e-3)*x+4.1665795894e-2)*x+1.6666665459e-1)*x + 5.0000001201e-1
	y = y*z + x + 1
	return math.Float32frombits(math.Float32bits(y) + uint32(int32(n))<<23)
}

// gelu is x·Φ(x), with erf from Abramowitz and Stegun 7.1.26 (absolute
// error under 1.5e-7, below float32's own at the values it meets).
func gelu(x float32) float32 {
	z := x * 0.70710678118654752
	a := z
	if a < 0 {
		a = -a
	}
	t := 1 / (1 + 0.3275911*a)
	e := 1 - (((((1.061405429*t-1.453152027)*t)+1.421413741)*t-0.284496736)*t+0.254829592)*t*expf(-a*a)
	if z < 0 {
		e = -e
	}
	return 0.5 * x * (1 + e)
}
