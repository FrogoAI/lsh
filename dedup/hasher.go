package dedup

import (
	"math"
	"math/rand"

	"github.com/cespare/xxhash/v2"

	"github.com/FrogoAI/set"
)

type Hasher struct {
	coefficientA []uint64
	coefficientB []uint64
	sigSize      int
}

func NewHasher(bands, rows int, seed int64) *Hasher {
	r := rand.New(rand.NewSource(seed)) //nolint:gosec

	sigSize := bands * rows

	a := make([]uint64, sigSize)
	b := make([]uint64, sigSize)

	for i := 0; i < sigSize; i++ {
		val := r.Uint64()
		if val%2 == 0 {
			val++
		}

		a[i] = val

		b[i] = r.Uint64()
	}

	return &Hasher{coefficientA: a, coefficientB: b, sigSize: sigSize}
}

func (h *Hasher) ComputeSignature(tokens set.GenericDataSet[string], sig []uint64) {
	for i := 0; i < h.sigSize; i++ {
		sig[i] = math.MaxUint64
	}

	for token := range tokens {
		hv := xxhash.Sum64String(token)
		for i := 0; i < h.sigSize; i++ {
			ph := h.coefficientA[i]*hv + h.coefficientB[i]

			if ph < sig[i] {
				sig[i] = ph
			}
		}
	}
}
