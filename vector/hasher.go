package vector

import "math/rand"

type Hasher struct {
	hyperplanes [][]float64
	sigSize     int
}

func NewHasher(bands, rows, dimensions int, seed int64) *Hasher {
	r := rand.New(rand.NewSource(seed)) //nolint:gosec

	sigSize := bands * rows

	hyperplanes := make([][]float64, sigSize)
	for i := 0; i < sigSize; i++ {
		plane := make([]float64, dimensions)
		for d := 0; d < dimensions; d++ {
			plane[d] = r.NormFloat64()
		}

		hyperplanes[i] = plane
	}

	return &Hasher{hyperplanes: hyperplanes, sigSize: sigSize}
}

// ComputeSignature produces a binary signature from a feature vector.
// Each bit = sign of dot product with a random hyperplane.
func (h *Hasher) ComputeSignature(vector []float64, sig []uint64) {
	for i := 0; i < h.sigSize; i++ {
		dot := 0.0
		for d := range vector {
			dot += h.hyperplanes[i][d] * vector[d]
		}

		if dot >= 0 {
			sig[i] = 1
		} else {
			sig[i] = 0
		}
	}
}
