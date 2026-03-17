package dedup

import (
	"strings"

	"github.com/FrogoAI/set"
)

func Shingle(input string, size int) set.GenericDataSet[string] {
	input = "^" + strings.ToLower(strings.TrimSpace(input)) + "$"

	runes := []rune(input)
	if len(runes) < size {
		return set.NewGenericDataSet[string](input)
	}

	tokenSet := set.NewGenericDataSet[string]()

	for i := 0; i <= len(runes)-size; i++ {
		token := string(runes[i : i+size])

		tokenSet.Add(token)
	}

	return tokenSet
}

func CalculateJaccardOptimized(sourceSet set.GenericDataSet[string], targetStr string, shingleSize int) float64 {
	sourceCount := sourceSet.Count()
	if sourceCount == 0 {
		return 0.0
	}

	targetSet := Shingle(targetStr, shingleSize)

	intersection := 0

	for token := range targetSet {
		if sourceSet.Contains(token) {
			intersection++
		}
	}

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
