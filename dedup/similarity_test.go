package dedup

import (
	"reflect"
	"testing"

	"github.com/FrogoAI/set"
)

func TestShingle(t *testing.T) {
	testcases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "normal sentence",
			input:    testHelloInput,
			expected: []string{"^he", "hel", "ell", "llo", "lo$"},
		},
		{
			name:     "exact size",
			input:    "cat",
			expected: []string{"^ca", "cat", "at$"},
		},
		{
			name:     "short input",
			input:    "hi",
			expected: []string{"^hi", "hi$"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{"^$"},
		},
		{
			name:     "whitespace trimming",
			input:    "  abc  ",
			expected: []string{"^ab", "abc", "bc$"},
		},
		{
			name:     "case insensitivity",
			input:    "AbC",
			expected: []string{"^ab", "abc", "bc$"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got := Shingle(tc.input, 3)

			if !reflect.DeepEqual(got, set.NewGenericDataSet[string](tc.expected...)) {
				t.Errorf("\ninput:    %q\nexpected: %v\ngot:      %v", tc.input, tc.expected, got)
			}
		})
	}
}

func TestEstimateJaccard(t *testing.T) {
	cases := []struct {
		name string
		sig1 []uint64
		sig2 []uint64
		want float64
	}{
		{name: "identical", sig1: []uint64{1, 2, 3}, sig2: []uint64{1, 2, 3}, want: 1.0},
		{name: "no match", sig1: []uint64{1, 2, 3}, sig2: []uint64{4, 5, 6}, want: 0.0},
		{name: "partial match", sig1: []uint64{1, 2, 3, 4}, sig2: []uint64{1, 2, 5, 6}, want: 0.5},
		{name: "empty slices", sig1: []uint64{}, sig2: []uint64{}, want: 0.0},
		{name: "different lengths", sig1: []uint64{1, 2}, sig2: []uint64{1}, want: 0.0},
		{name: "nil sig1", sig1: nil, sig2: []uint64{1}, want: 0.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateJaccard(tc.sig1, tc.sig2)
			if got != tc.want {
				t.Errorf("got %f, want %f", got, tc.want)
			}
		})
	}
}

func TestCalculateJaccardOptimized(t *testing.T) {
	tests := []struct {
		name      string
		sourceStr string
		targetStr string
		want      float64
	}{
		{name: "exact match", sourceStr: testHelloInput, targetStr: testHelloInput, want: 1.0},
		{name: "completely different", sourceStr: "aaaaa", targetStr: "bbbbb", want: 0.0},
		{name: "partial overlap", sourceStr: "context", targetStr: "content", want: 0.6666666666666666},
		{name: "empty source", sourceStr: "", targetStr: "anything", want: 0.0},
		{name: "subset", sourceStr: "masterpiece", targetStr: "master", want: 0.46153846153846156},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sourceSet := Shingle(tc.sourceStr, 2)
			got := CalculateJaccardOptimized(sourceSet, tc.targetStr, 2)

			if got != tc.want {
				t.Errorf("got %f, want %f", got, tc.want)
			}
		})
	}
}
