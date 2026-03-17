package dedup

import (
	"math/rand"
	"testing"

	"github.com/cespare/xxhash/v2"

	"github.com/FrogoAI/set"
)

func TestSig0_Deterministic(t *testing.T) {
	seed := int64(1)
	bands := 10
	rows := 2
	input := "hello"

	hasher := NewHasher(bands, rows, seed)

	r := rand.New(rand.NewSource(seed)) //nolint:gosec

	valA := r.Uint64()
	if valA%2 == 0 {
		valA++
	}

	expectedA := valA
	expectedB := r.Uint64()

	hv := xxhash.Sum64String(input)
	expectedSig0 := expectedA*hv + expectedB

	sig := make([]uint64, 100)
	tokens := []string{input}

	hasher.ComputeSignature(set.NewGenericDataSet[string](tokens...), sig)

	if sig[0] != expectedSig0 {
		t.Fatalf("\nmath mismatch!\n"+
			"seed:      %d\n"+
			"input:     %s\n"+
			"coeff A:   %d\n"+
			"coeff B:   %d\n"+
			"xxhash:    %d\n"+
			"expected:  %d\n"+
			"actual:    %d\n",
			seed, input, expectedA, expectedB, hv, expectedSig0, sig[0])
	}
}
