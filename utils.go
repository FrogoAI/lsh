package lsh

import (
	"strings"

	"github.com/FrogoAI/set"
)

func (s *SimilarityService) Shingle(input string) set.GenericDataSet[string] {
	input = "^" + strings.ToLower(strings.TrimSpace(input)) + "$"

	runes := []rune(input)
	if len(runes) < s.config.ShingleSize {
		return set.NewGenericDataSet[string](input)
	}

	tokenSet := set.NewGenericDataSet[string]()

	for i := 0; i <= len(runes)-s.config.ShingleSize; i++ {
		token := string(runes[i : i+s.config.ShingleSize])

		tokenSet.Add(token)
	}

	return tokenSet
}

func (s *SimilarityService) CalculateJaccardOptimized(sourceSet set.GenericDataSet[string], targetStr string) float64 {
	sourceCount := sourceSet.Count()
	if sourceCount == 0 {
		return 0.0
	}

	targetSet := s.Shingle(targetStr)

	intersection := 0

	for token := range targetSet {
		if sourceSet.Contains(token) {
			intersection++
		}
	}

	// |A ∪ B| = |A| + |B| - |A ∩ B|
	union := sourceCount + targetSet.Count() - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
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
