#include "textflag.h"

// func dot4sdot(a, b *int8, k int, out *[4]int32)
// out[j] = Σ a[i]*b[j*k+i] for i < k, k a multiple of 32, with SDOT
// (ARMv8.2 dot product; encoded by hand, the Go assembler has no name
// for it).
TEXT ·dot4sdot(SB), NOSPLIT, $0-32
	MOVD a+0(FP), R0
	MOVD b+8(FP), R1
	MOVD k+16(FP), R2
	MOVD out+24(FP), R3
	ADD  R2, R1, R4
	ADD  R2, R4, R5
	ADD  R2, R5, R6
	VEOR V0.B16, V0.B16, V0.B16
	VEOR V1.B16, V1.B16, V1.B16
	VEOR V2.B16, V2.B16, V2.B16
	VEOR V3.B16, V3.B16, V3.B16
	VEOR V4.B16, V4.B16, V4.B16
	VEOR V5.B16, V5.B16, V5.B16
	VEOR V6.B16, V6.B16, V6.B16
	VEOR V7.B16, V7.B16, V7.B16

loop:
	VLD1.P 32(R0), [V8.B16, V9.B16]
	VLD1.P 32(R1), [V10.B16, V11.B16]
	VLD1.P 32(R4), [V12.B16, V13.B16]
	VLD1.P 32(R5), [V14.B16, V15.B16]
	VLD1.P 32(R6), [V16.B16, V17.B16]
	WORD $0x4e8a9500 // SDOT V0.4S, V8.16B, V10.16B
	WORD $0x4e8b9524 // SDOT V4.4S, V9.16B, V11.16B
	WORD $0x4e8c9501 // SDOT V1.4S, V8.16B, V12.16B
	WORD $0x4e8d9525 // SDOT V5.4S, V9.16B, V13.16B
	WORD $0x4e8e9502 // SDOT V2.4S, V8.16B, V14.16B
	WORD $0x4e8f9526 // SDOT V6.4S, V9.16B, V15.16B
	WORD $0x4e909503 // SDOT V3.4S, V8.16B, V16.16B
	WORD $0x4e919527 // SDOT V7.4S, V9.16B, V17.16B
	SUBS $32, R2, R2
	BNE  loop

	VADD  V4.S4, V0.S4, V0.S4
	VADD  V5.S4, V1.S4, V1.S4
	VADD  V6.S4, V2.S4, V2.S4
	VADD  V7.S4, V3.S4, V3.S4
	VADDV V0.S4, V0
	VADDV V1.S4, V1
	VADDV V2.S4, V2
	VADDV V3.S4, V3
	VMOV  V0.S[0], R7
	MOVW  R7, 0(R3)
	VMOV  V1.S[0], R7
	MOVW  R7, 4(R3)
	VMOV  V2.S[0], R7
	MOVW  R7, 8(R3)
	VMOV  V3.S[0], R7
	MOVW  R7, 12(R3)
	RET

// func scoresT(q, kt *float32, np int, out *float32)
// out[u] = Σ q[i]*kt[i*np+u] for i < 32, u < np, np a multiple of 16.
TEXT ·scoresT(SB), NOSPLIT, $0-32
	MOVD q+0(FP), R0
	MOVD kt+8(FP), R1
	MOVD np+16(FP), R2
	MOVD out+24(FP), R3
	LSL  $2, R2, R4 // a row of kt in bytes
	CBZ  R2, tdone
tblock:
	VEOR V0.B16, V0.B16, V0.B16
	VEOR V1.B16, V1.B16, V1.B16
	VEOR V2.B16, V2.B16, V2.B16
	VEOR V3.B16, V3.B16, V3.B16
	MOVD R0, R5
	MOVD R1, R6
	MOVD $32, R7
trow:
	VLD1R.P 4(R5), [V16.S4]
	VLD1    (R6), [V8.S4, V9.S4, V10.S4, V11.S4]
	ADD     R4, R6, R6
	VFMLA   V16.S4, V8.S4, V0.S4
	VFMLA   V16.S4, V9.S4, V1.S4
	VFMLA   V16.S4, V10.S4, V2.S4
	VFMLA   V16.S4, V11.S4, V3.S4
	SUB     $1, R7
	CBNZ    R7, trow
	VST1.P  [V0.S4, V1.S4, V2.S4, V3.S4], 64(R3)
	ADD     $64, R1
	SUB     $16, R2
	CBNZ    R2, tblock
tdone:
	RET

// func mix32(c, p, v *float32, n int)
// c[i] += Σ p[u]*v[u*32+i] for u < n, i < 32.
TEXT ·mix32(SB), NOSPLIT, $0-32
	MOVD  c+0(FP), R0
	MOVD  p+8(FP), R1
	MOVD  v+16(FP), R2
	MOVD  n+24(FP), R3
	VLD1  (R0), [V0.S4, V1.S4, V2.S4, V3.S4]
	ADD   $64, R0, R4
	VLD1  (R4), [V4.S4, V5.S4, V6.S4, V7.S4]
	CBZ   R3, mdone
mloop:
	VLD1R.P 4(R1), [V16.S4]
	VLD1.P 64(R2), [V8.S4, V9.S4, V10.S4, V11.S4]
	VLD1.P 64(R2), [V12.S4, V13.S4, V14.S4, V15.S4]
	VFMLA V16.S4, V8.S4, V0.S4
	VFMLA V16.S4, V9.S4, V1.S4
	VFMLA V16.S4, V10.S4, V2.S4
	VFMLA V16.S4, V11.S4, V3.S4
	VFMLA V16.S4, V12.S4, V4.S4
	VFMLA V16.S4, V13.S4, V5.S4
	VFMLA V16.S4, V14.S4, V6.S4
	VFMLA V16.S4, V15.S4, V7.S4
	SUB   $1, R3
	CBNZ  R3, mloop
mdone:
	VST1  [V0.S4, V1.S4, V2.S4, V3.S4], (R0)
	VST1  [V4.S4, V5.S4, V6.S4, V7.S4], (R4)
	RET
