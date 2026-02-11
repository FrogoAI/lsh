package lsh

import (
	"math"
	"math/rand"
	"strconv"

	"github.com/cespare/xxhash/v2"
)

type Hasher struct {
	coefficientA []uint64
	coefficientB []uint64

	bands int
	rows  int
}

func NewHasher(bands, rows int, seed int64) *Hasher {
	r := rand.New(rand.NewSource(seed))

	a := make([]uint64, SignatureSize)
	b := make([]uint64, SignatureSize)

	for i := 0; i < SignatureSize; i++ {
		val := r.Uint64()
		if val%2 == 0 {
			val++ // we must have only odd values for next step
		}
		a[i] = val

		b[i] = r.Uint64()
	}

	return &Hasher{coefficientA: a, coefficientB: b, bands: bands, rows: rows}
}

func (h *Hasher) ComputeSignature(tokens []string, sig []uint64) {
	for i := 0; i < SignatureSize; i++ {
		sig[i] = math.MaxUint64
	}

	for _, token := range tokens {
		hv := xxhash.Sum64String(token)
		for i := 0; i < SignatureSize; i++ {
			// Permutation: (a*h + b) % 2^64
			// Since we ensured 'a' is odd, this is mathematically correct.
			ph := h.coefficientA[i]*hv + h.coefficientB[i]

			if ph < sig[i] {
				sig[i] = ph
			}
		}
	}
}

func (h *Hasher) ComputeBands(signature []uint64) ([]string, error) {
	if len(signature) < h.bands*h.rows {
		return nil, ErrSignatureTooShort
	}

	bucketKeys := make([]string, h.bands)
	buf := make([]byte, h.rows*8) // Reusable buffer

	for i := 0; i < h.bands; i++ {
		start := i * h.rows

		offset := 0
		for k := 0; k < h.rows; k++ {
			val := signature[start+k]
			buf[offset] = byte(val)
			buf[offset+1] = byte(val >> 8)
			buf[offset+2] = byte(val >> 16)
			buf[offset+3] = byte(val >> 24)
			buf[offset+4] = byte(val >> 32)
			buf[offset+5] = byte(val >> 40)
			buf[offset+6] = byte(val >> 48)
			buf[offset+7] = byte(val >> 56)
			offset += 8
		}

		d := xxhash.New()
		_, err := d.Write(buf)
		if err != nil {
			return nil, err
		}

		bucketKeys[i] = "b" + strconv.Itoa(i) + ":" + strconv.FormatUint(d.Sum64(), 16)
	}

	return bucketKeys, nil
}
