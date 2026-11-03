//go:build goexperiment.simd && (arm64 || wasm)

package terminal

import "simd/archsimd"

// changedBits sets m's words for the n cells of a and b, as cellWords, a bit a
// cell that differs, the bits past n clear. NEON or wasm SIMD128 take four cells
// in three 128-bit compares, and four alike cost two lane reads; on arm64 a block
// of 64 that one memcmp finds unchanged is skipped first.
func changedBits(m []uint64, a, b []uint32, n int) {
	full := n >> 6
	for w := range full {
		x, y := (*[192]uint32)(a[192*w:]), (*[192]uint32)(b[192*w:])
		var bits uint64
		if !skipEqual || *x != *y {
			for g := range 16 {
				p, q := (*[12]uint32)(x[12*g:]), (*[12]uint32)(y[12*g:])
				v0 := archsimd.LoadUint32x4Array((*[4]uint32)(p[0:4])).Xor(archsimd.LoadUint32x4Array((*[4]uint32)(q[0:4]))).ReshapeToUint64s()
				v1 := archsimd.LoadUint32x4Array((*[4]uint32)(p[4:8])).Xor(archsimd.LoadUint32x4Array((*[4]uint32)(q[4:8]))).ReshapeToUint64s()
				v2 := archsimd.LoadUint32x4Array((*[4]uint32)(p[8:12])).Xor(archsimd.LoadUint32x4Array((*[4]uint32)(q[8:12]))).ReshapeToUint64s()
				if s := v0.Or(v1).Or(v2); s.GetElem(0)|s.GetElem(1) == 0 {
					continue
				}
				// Lane l holds words 2l and 2l+1 of the twelve, cell k in 3k to 3k+2
				l0, l1, l2 := v0.GetElem(0), v0.GetElem(1), v1.GetElem(0)
				l3, l4, l5 := v1.GetElem(1), v2.GetElem(0), v2.GetElem(1)
				c := nonzero(l0|l1<<32) | nonzero(l1>>32|l2)<<1 | nonzero(l3|l4<<32)<<2 | nonzero(l4>>32|l5)<<3
				bits |= c << (4 * g)
			}
		}
		m[w] = bits
	}
	changedWords(m, a, b, n, full)
}

// nonzero is 1 for a v other than 0, else 0
func nonzero(v uint64) uint64 { return (v | -v) >> 63 }
