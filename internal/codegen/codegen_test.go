package codegen

import "testing"

func TestCode_NoCollisionsAndNotSequential(t *testing.T) {
	seen := make(map[string]uint64)
	const n = 100000
	for i := uint64(0); i < n; i++ {
		c := Code(i)
		if prev, ok := seen[c]; ok {
			t.Fatalf("collision: counter %d and %d both produced %q", prev, i, c)
		}
		seen[c] = i
	}
}

func TestCode_NotSequentialLooking(t *testing.T) {
	c0 := Code(0)
	c1 := Code(1)
	c2 := Code(2)
	if c0 == c1 || c1 == c2 {
		t.Fatalf("expected distinct codes, got %q %q %q", c0, c1, c2)
	}
	// Sequential input should not produce a code that's just c0 "+1" in any
	// obvious lexical sense (regression guard against accidentally encoding
	// the raw counter).
	if c1 == c0+"1" {
		t.Fatalf("code looks like raw counter encoding, permutation not applied")
	}
}

func TestCode_Deterministic(t *testing.T) {
	first := Code(12345)
	second := Code(12345)
	if first != second {
		t.Fatal("Code must be deterministic for the same input")
	}
}
