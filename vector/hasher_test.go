package vector

import "testing"

func TestVectorHasher_Deterministic(t *testing.T) {
	h1 := NewHasher(5, 3, 4, 42)
	h2 := NewHasher(5, 3, 4, 42)

	vec := []float64{1.0, 2.0, 3.0, 4.0}

	sig1 := make([]uint64, 15)
	sig2 := make([]uint64, 15)

	h1.ComputeSignature(vec, sig1)
	h2.ComputeSignature(vec, sig2)

	for i := range sig1 {
		if sig1[i] != sig2[i] {
			t.Fatalf("non-deterministic: sig1[%d]=%d, sig2[%d]=%d", i, sig1[i], i, sig2[i])
		}
	}
}

func TestVectorHasher_BinarySignature(t *testing.T) {
	cases := []struct {
		name string
		vec  []float64
	}{
		{name: "mixed sign", vec: []float64{1.0, -2.0, 3.0, -4.0}},
		{name: "all positive", vec: []float64{1.0, 2.0, 3.0, 4.0}},
		{name: "all negative", vec: []float64{-1.0, -2.0, -3.0, -4.0}},
		{name: "all zeros", vec: []float64{0.0, 0.0, 0.0, 0.0}},
		{name: "very large", vec: []float64{1e15, 1e15, 1e15, 1e15}},
		{name: "very small", vec: []float64{1e-15, 1e-15, 1e-15, 1e-15}},
	}

	h := NewHasher(5, 3, 4, 42)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig := make([]uint64, 15)
			h.ComputeSignature(tc.vec, sig)

			for i, v := range sig {
				if v != 0 && v != 1 {
					t.Errorf("sig[%d] = %d, expected 0 or 1", i, v)
				}
			}
		})
	}
}

func TestVectorHasher_SimilarVectorsSimilarSigs(t *testing.T) {
	h := NewHasher(20, 5, 4, 42)

	v1 := []float64{1.0, 2.0, 3.0, 4.0}
	v2 := []float64{1.1, 2.1, 3.1, 4.1}     // very close
	v3 := []float64{-4.0, -3.0, -2.0, -1.0} // very different

	sig1 := make([]uint64, 100)
	sig2 := make([]uint64, 100)
	sig3 := make([]uint64, 100)

	h.ComputeSignature(v1, sig1)
	h.ComputeSignature(v2, sig2)
	h.ComputeSignature(v3, sig3)

	matchClose := 0
	matchFar := 0

	for i := range sig1 {
		if sig1[i] == sig2[i] {
			matchClose++
		}

		if sig1[i] == sig3[i] {
			matchFar++
		}
	}

	if matchClose <= matchFar {
		t.Errorf("expected more matches for close vectors (%d) than far vectors (%d)", matchClose, matchFar)
	}
}

func TestVectorHasher_DifferentSeedsDiffer(t *testing.T) {
	h1 := NewHasher(5, 3, 4, 1)
	h2 := NewHasher(5, 3, 4, 2)

	vec := []float64{1.0, 2.0, 3.0, 4.0}
	sig1 := make([]uint64, 15)
	sig2 := make([]uint64, 15)

	h1.ComputeSignature(vec, sig1)
	h2.ComputeSignature(vec, sig2)

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

func TestVectorHasher_OrthogonalVectorsLowMatch(t *testing.T) {
	h := NewHasher(20, 5, 4, 42)

	v1 := []float64{1.0, 0.0, 0.0, 0.0}
	v2 := []float64{0.0, 0.0, 0.0, 1.0}

	sig1 := make([]uint64, 100)
	sig2 := make([]uint64, 100)

	h.ComputeSignature(v1, sig1)
	h.ComputeSignature(v2, sig2)

	matches := 0

	for i := range sig1 {
		if sig1[i] == sig2[i] {
			matches++
		}
	}

	// Orthogonal vectors: expected ~50% match (random coin flip)
	if matches > 70 || matches < 30 {
		t.Errorf("orthogonal vectors: expected ~50%% match, got %d/100", matches)
	}
}

func TestVectorHasher_OppositeVectorsLowMatch(t *testing.T) {
	h := NewHasher(20, 5, 4, 42)

	v1 := []float64{1.0, 2.0, 3.0, 4.0}
	v2 := []float64{-1.0, -2.0, -3.0, -4.0}

	sig1 := make([]uint64, 100)
	sig2 := make([]uint64, 100)

	h.ComputeSignature(v1, sig1)
	h.ComputeSignature(v2, sig2)

	matches := 0

	for i := range sig1 {
		if sig1[i] == sig2[i] {
			matches++
		}
	}

	// Opposite vectors: expected ~0% match
	if matches > 15 {
		t.Errorf("opposite vectors: expected very low match, got %d/100", matches)
	}
}
