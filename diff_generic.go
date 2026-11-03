//go:build !goexperiment.simd || !(amd64 || arm64 || wasm)

package terminal

// changedBits sets m's words for the n cells of a and b, as cellWords, a bit a
// cell that differs, the bits past n clear
func changedBits(m []uint64, a, b []uint32, n int) { changedWords(m, a, b, n, 0) }
