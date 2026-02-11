package lsh

import (
	"reflect"
	"testing"

	"github.com/FrogoAI/set"
	"github.com/FrogoAI/testutils"
)

func TestShingle(t *testing.T) {
	service := &SimilarityService{
		config: &Config{ShingleSize: 3},
	}

	testcases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Normal Sentence",
			input:    "hello",
			expected: []string{"hel", "ell", "llo"},
		},
		{
			name:     "Exact Size",
			input:    "cat",
			expected: []string{"cat"},
		},
		{
			name:     "Short Input (Keep Original)",
			input:    "hi",
			expected: []string{"hi"},
		},
		{
			name:     "Empty String",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "Whitespace Trimming",
			input:    "  abc  ",
			expected: []string{"abc"},
		},
		{
			name:     "Case Insensitivity",
			input:    "AbC",
			expected: []string{"abc"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expected == nil {
				t.Skip("Skipping undefined expectation")
			}

			got := service.Shingle(tc.input)

			if !reflect.DeepEqual(got, set.NewGenericDataSet[string](tc.expected...)) {
				t.Errorf("\nInput:    %q\nExpected: %v\nGot:      %v", tc.input, tc.expected, got)
			}
		})
	}
}

func TestCalculateJaccardOptimized(t *testing.T) {
	service := &SimilarityService{
		config: &Config{ShingleSize: 2},
	}

	tests := []struct {
		name      string
		sourceStr string
		targetStr string
		want      float64
	}{
		{
			name:      "Exact Match",
			sourceStr: "hello",
			targetStr: "hello",
			want:      1.0,
		},
		{
			name:      "Completely Different",
			sourceStr: "aaaaa",
			targetStr: "bbbbb",
			want:      0.0,
		},
		{
			name:      "Partial Overlap (Half)",
			sourceStr: "context", // {co, on, nt, te, ex, xt} (6)
			targetStr: "content", // {co, on, nt, te, en}     (5 unique)
			want:      4.0 / 7.0,
		},
		{
			name:      "Empty Source Set",
			sourceStr: "",
			targetStr: "anything",
			want:      0.0,
		},
		{
			name:      "Subset (Target inside Source)",
			sourceStr: "masterpiece",
			targetStr: "master",
			want:      0.5,
		},
		{
			name:      "Order Independence",
			sourceStr: "ab cd ef",
			targetStr: "ef ab cd",
			want:      5.0 / 9.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sourceSet := service.Shingle(tc.sourceStr)

			got := service.CalculateJaccardOptimized(sourceSet, tc.targetStr)

			testutils.Equal(t, got, tc.want)
		})
	}
}
