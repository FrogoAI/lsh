package lsh

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"strings"
	"time"

	"github.com/FrogoAI/set"
)

func (s *SimilarityService) Shingle(input string) set.GenericDataSet[string] {
	input = "^" + strings.ToLower(strings.TrimSpace(input)) + "$"

	if len(input) < s.config.ShingleSize {
		return set.NewGenericDataSet[string](input)
	}

	tokenSet := set.NewGenericDataSet[string]()

	for i := 0; i <= len(input)-s.config.ShingleSize; i++ {
		token := input[i : i+s.config.ShingleSize]

		tokenSet.Add(token)
	}

	return tokenSet
}

func (s *SimilarityService) CalculateJaccardOptimized(sourceSet set.GenericDataSet[string], targetStr string) float64 {
	if sourceSet.Count() == 0 {
		return 0.0
	}

	targetSet := s.Shingle(targetStr)

	intersection := float64(targetSet.Intersection(sourceSet).Count())
	union := float64(targetSet.Union(sourceSet).Count())

	if union == 0 {
		return 0.0
	}

	return intersection / union
}

func GetTinyID() ([]byte, error) {
	b := make([]byte, 4) //nolint:mnd

	_, err := rand.Read(b) //nolint:gosec
	if err != nil {
		return nil, err
	}

	r := make([]byte, 4) //nolint:mnd
	// time.Now().UnixNano()
	binary.BigEndian.PutUint32(r, uint32(time.Now().Nanosecond()))
	b = append(b, r...)
	val := binary.BigEndian.Uint64(b)

	return []byte(big.NewInt(int64(val)).Text(62))[5:], nil //nolint:mnd
}

func EstimateJaccard(sig1, sig2 []uint64) float64 {
	if len(sig1) == 0 || len(sig1) != len(sig2) {
		return 0.0
	}

	matches := 0
	for i := range sig1 {
		if sig1[i] == sig2[i] {
			matches++
		}
	}

	return float64(matches) / float64(len(sig1))
}
