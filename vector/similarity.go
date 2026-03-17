package vector

import "math"

// EstimateCosine estimates cosine similarity from two binary (hyperplane) signatures.
// P(same sign) = 1 - angle/pi → cosine = cos(pi * (1 - matchFraction)).
func EstimateCosine(sig1, sig2 []uint64) float64 {
	if len(sig1) == 0 || len(sig1) != len(sig2) {
		return 0.0
	}

	matches := 0

	for i := range sig1 {
		if sig1[i] == sig2[i] {
			matches++
		}
	}

	matchFraction := float64(matches) / float64(len(sig1))

	return math.Cos(math.Pi * (1 - matchFraction))
}

// ExactCosine computes the exact cosine similarity between two vectors.
func ExactCosine(v1, v2 []float64) float64 {
	if len(v1) != len(v2) || len(v1) == 0 {
		return 0.0
	}

	var dot, norm1, norm2 float64

	for i := range v1 {
		dot += v1[i] * v2[i]
		norm1 += v1[i] * v1[i]
		norm2 += v2[i] * v2[i]
	}

	denom := math.Sqrt(norm1) * math.Sqrt(norm2)
	if denom == 0 {
		return 0.0
	}

	return dot / denom
}
