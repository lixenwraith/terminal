//go:build goexperiment.simd

package terminal

import "simd/archsimd"

var avx2 = archsimd.X86.AVX2()

// changedBits sets m's words for the n cells of a and b, as cellWords, a bit a
// cell that differs, the bits past n clear. With AVX2, a block of 64 cells one
// memcmp finds changed takes 24 compares of eight words, each cell's three
// folding to its bit.
func changedBits(m []uint64, a, b []uint32, n int) {
	if !avx2 {
		changedWords(m, a, b, n, 0)
		return
	}
	full := n >> 6
	for w := range full {
		x, y := (*[192]uint32)(a[192*w:]), (*[192]uint32)(b[192*w:])
		var bits uint64
		if *x != *y {
			for g := range 8 {
				p, q := (*[24]uint32)(x[24*g:]), (*[24]uint32)(y[24*g:])
				e := uint32(archsimd.LoadUint32x8Array((*[8]uint32)(p[0:8])).Equal(archsimd.LoadUint32x8Array((*[8]uint32)(q[0:8]))).ToBits()) |
					uint32(archsimd.LoadUint32x8Array((*[8]uint32)(p[8:16])).Equal(archsimd.LoadUint32x8Array((*[8]uint32)(q[8:16]))).ToBits())<<8 |
					uint32(archsimd.LoadUint32x8Array((*[8]uint32)(p[16:24])).Equal(archsimd.LoadUint32x8Array((*[8]uint32)(q[16:24]))).ToBits())<<16
				bits |= uint64(cellBits8(^e)) << (8 * g)
			}
		}
		m[w] = bits
	}
	changedWords(m, a, b, n, full)
}

// cellBits8 folds the differing words of eight cells, three bits a cell, into
// a bit a cell
func cellBits8(d uint32) uint8 {
	d = (d | d>>1 | d>>2) & 0x249249
	d = (d | d>>2) & 0x0c30c3
	d = (d | d>>4) & 0x00f00f
	return uint8(d | d>>8)
}
