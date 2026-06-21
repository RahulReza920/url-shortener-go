// Package codegen turns a monotonic counter into a short, non-sequential,
// collision-free public code.
//
// Uniqueness comes from a Redis INCR counter — never reused, never retried.
// Unguessability comes from running the counter through a reversible bit
// permutation over a 42-bit space before base62-encoding it, so code(n) and
// code(n+1) don't look related. The permutation is a composition of XOR,
// odd-multiplier multiplication (invertible mod 2^42), and rotation — each
// step is a bijection, so the composition is too: every counter value maps
// to exactly one code and no two counter values collide.
//
// The 42-bit space supports ~4.4 trillion codes before the counter would
// wrap and start re-issuing scrambled values that were already handed out;
// that ceiling is treated as a non-issue for v1.
package codegen

const (
	spaceBits = 42
	mask      = (uint64(1) << spaceBits) - 1

	xorConst = 0x2B5C4F1A9E3
	mulConst = 0x1B873593F61 // odd, so it's invertible mod 2^42

	rotateBy = 13

	alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	base     = uint64(len(alphabet))
)

// Code returns the public short code for counter value n.
func Code(n uint64) string {
	return base62Encode(permute(n))
}

func permute(n uint64) uint64 {
	n &= mask
	n ^= xorConst
	n = (n * mulConst) & mask
	n = rotateLeft(n, rotateBy)
	return n
}

func rotateLeft(n uint64, k uint) uint64 {
	k %= spaceBits
	return ((n << k) | (n >> (spaceBits - k))) & mask
}

func base62Encode(n uint64) string {
	if n == 0 {
		return string(alphabet[0])
	}
	var buf [11]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = alphabet[n%base]
		n /= base
	}
	return string(buf[i:])
}
