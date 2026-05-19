package dedup

import (
	"math"
	"math/rand"
	"testing"

	"github.com/cespare/xxhash/v2"

	"github.com/FrogoAI/set"
)

func TestSig0_Deterministic(t *testing.T) {
	seed := int64(1)
	bands := 10
	rows := 2
	input := testHelloInput

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

func TestComputeSignature_EdgeCases(t *testing.T) {
	cases := []struct {
		name   string
		tokens []string
		check  func(t *testing.T, sig []uint64)
	}{
		{
			name:   "single token",
			tokens: []string{"a"},
			check: func(t *testing.T, sig []uint64) {
				for i, v := range sig {
					if v == math.MaxUint64 {
						t.Errorf("sig[%d] still MaxUint64 after single token", i)
					}
				}
			},
		},
		{
			name:   "duplicate tokens are deduplicated by set",
			tokens: []string{"a", "a", "a"},
			check: func(t *testing.T, _ []uint64) {
				h := NewHasher(5, 2, 42)
				sigSingle := make([]uint64, 10)
				h.ComputeSignature(set.NewGenericDataSet[string]("a"), sigSingle)

				h2 := NewHasher(5, 2, 42)
				sigDup := make([]uint64, 10)
				h2.ComputeSignature(set.NewGenericDataSet[string]("a", "a", "a"), sigDup)

				for i := range sigSingle {
					if sigSingle[i] != sigDup[i] {
						t.Errorf("duplicate tokens changed sig[%d]: %d vs %d", i, sigSingle[i], sigDup[i])
					}
				}
			},
		},
		{
			name:   "empty token set leaves maxuint64",
			tokens: []string{},
			check: func(t *testing.T, sig []uint64) {
				for i, v := range sig {
					if v != math.MaxUint64 {
						t.Errorf("sig[%d] = %d, expected MaxUint64 for empty input", i, v)
					}
				}
			},
		},
		{
			name:   "many tokens all update signature",
			tokens: []string{"alpha", "beta", "gamma", "delta", "epsilon"},
			check: func(t *testing.T, sig []uint64) {
				for i, v := range sig {
					if v == math.MaxUint64 {
						t.Errorf("sig[%d] still MaxUint64 after 5 tokens", i)
					}
				}
			},
		},
		{
			name:   "unicode tokens",
			tokens: []string{"привет", "мир", "日本語"},
			check: func(t *testing.T, sig []uint64) {
				for i, v := range sig {
					if v == math.MaxUint64 {
						t.Errorf("sig[%d] still MaxUint64 for unicode tokens", i)
					}
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHasher(5, 2, 42)
			sig := make([]uint64, 10)
			h.ComputeSignature(set.NewGenericDataSet[string](tc.tokens...), sig)
			tc.check(t, sig)
		})
	}
}

func TestComputeSignature_SameSeedDeterministic(t *testing.T) {
	h1 := NewHasher(10, 3, 99)
	h2 := NewHasher(10, 3, 99)

	tokens := set.NewGenericDataSet[string](testHelloInput, "world")
	sig1 := make([]uint64, 30)
	sig2 := make([]uint64, 30)

	h1.ComputeSignature(tokens, sig1)
	h2.ComputeSignature(tokens, sig2)

	for i := range sig1 {
		if sig1[i] != sig2[i] {
			t.Fatalf("non-deterministic at sig[%d]: %d vs %d", i, sig1[i], sig2[i])
		}
	}
}

func TestComputeSignature_DifferentSeedsDiffer(t *testing.T) {
	h1 := NewHasher(10, 3, 1)
	h2 := NewHasher(10, 3, 2)

	tokens := set.NewGenericDataSet[string](testHelloInput, "world")
	sig1 := make([]uint64, 30)
	sig2 := make([]uint64, 30)

	h1.ComputeSignature(tokens, sig1)
	h2.ComputeSignature(tokens, sig2)

	allSame := true

	for i := range sig1 {
		if sig1[i] != sig2[i] {
			allSame = false

			break
		}
	}

	if allSame {
		t.Error("different seeds produced identical signatures")
	}
}
