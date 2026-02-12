package lsh

import (
	"math/rand"
	"testing"

	"github.com/cespare/xxhash/v2"

	"github.com/FrogoAI/set"
	"github.com/FrogoAI/testutils"
)

func TestSig0_Deterministic(t *testing.T) {
	seed := int64(1)
	bands := 10
	rows := 2
	input := "hello"

	lsh := NewHasher(bands, rows, seed)

	r := rand.New(rand.NewSource(seed)) // nolint:gosec

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

	lsh.ComputeSignature(set.NewGenericDataSet[string](tokens...), sig)

	if sig[0] != expectedSig0 {
		t.Fatalf("\nMath Mismatch!\n"+
			"Seed:      %d\n"+
			"Input:     %s\n"+
			"Coeff A:   %d\n"+
			"Coeff B:   %d\n"+
			"XXHash:    %d\n"+
			"----------------------------------\n"+
			"Expected:  %d  (Manual Calc)\n"+
			"Actual:    %d  (Code Output)\n",
			seed, input, expectedA, expectedB, hv, expectedSig0, sig[0])
	}

	t.Logf("Success! sig[0] matched expected value: %d", sig[0])
}

func TestComputeBands_TableDriven(t *testing.T) {
	bands := 3
	rows := 2
	hasher := &Hasher{bands: bands, rows: rows, sigSize: bands * rows}

	testcases := []struct {
		name      string
		signature []uint64
		check     func(t *testing.T, res []string, err error)
	}{
		{
			name:      "Happy Path: Standard Signature",
			signature: []uint64{1, 1, 2, 2, 3, 3},
			check: func(t *testing.T, res []string, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				if len(res) != 3 {
					t.Errorf("Expected 3 band keys, got %d", len(res))
				}

				if res[0] != "b0:ed7aead4b89ab6fc" {
					t.Errorf("Band 0 malformed: %s", res[0])
				}

				if res[2] != "b2:75ec20e3cf098126" {
					t.Errorf("Band 2 malformed: %s", res[2])
				}
			},
		},
		{
			name:      "Edge Case: All Zeros",
			signature: []uint64{0, 0, 0, 0, 0, 0},
			check: func(t *testing.T, res []string, err error) {
				if err != nil {
					t.Fatal(err)
				}

				if len(res) != 3 {
					t.Error("Zeros signature produced wrong number of bands")
				}

				if res[0] == res[1] {
					t.Error("Bands collided! Prefixes (b0 vs b1) failed to separate identical data.")
				}
			},
		},
		{
			name:      "Edge Case: Short Signature (Error Expected)",
			signature: []uint64{1, 2, 3},
			check: func(t *testing.T, _ []string, err error) {
				testutils.Equal(t, err, ErrSignatureTooShort)
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := hasher.ComputeBands(tc.signature)
			tc.check(t, res, err)
		})
	}
}
