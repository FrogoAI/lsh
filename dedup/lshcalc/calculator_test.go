package lshcalc

import (
	"testing"

	"github.com/FrogoAI/testutils"
)

func TestCalculator(t *testing.T) {
	result := [][]float64{
		{25, 2},
		{23, 2},
		{22, 2},
		{21, 2},
		{21, 2},
		{38, 3},
		{36, 3},
		{34, 3},
		{32, 3},
		{31, 3},
		{29, 3},
		{27, 3},
		{26, 3},
		{25, 3},
		{38, 4},
		{36, 4},
		{33, 4},
		{31, 4},
		{29, 4},
		{27, 4},
		{38, 5},
		{35, 5},
		{33, 5},
		{30, 5},
		{39, 6},
		{36, 6},
		{33, 6},
		{40, 7},
		{36, 7},
		{33, 7},
		{38, 8},
		{34, 8},
		{38, 9},
		{34, 9},
		{36, 10},
		{32, 10},
		{28, 10},
		{25, 10},
		{22, 10},
		{19, 10},
	}

	j := 0
	for i := float64(50); i < 90; i++ {
		res, err := Calculate(Request{
			Jaccard:     i,
			ShingleSize: 2,
			ErrorRate:   0.001,
		})
		if err != nil {
			t.Fatalf("calculate: %v", err)
		}

		vals := result[j]

		testutils.Equal(t, int(vals[0]), res.Bands)
		testutils.Equal(t, int(vals[1]), res.Rows)

		j++
	}
}

func TestCalculateAcceptsPercentInputs(t *testing.T) {
	t.Parallel()

	result, err := Calculate(Request{
		Jaccard:     70,
		ShingleSize: 3,
		ErrorRate:   0.6,
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}

	if result.Bands != 28 {
		t.Fatalf("bands = %d, want 28", result.Bands)
	}

	if result.Rows != 5 {
		t.Fatalf("rows = %d, want 5", result.Rows)
	}

	if result.MissProbability > 0.006 {
		t.Fatalf("miss probability = %f, want <= 0.006", result.MissProbability)
	}
}

func TestCalculateUsesShingleSizeAsOutputDimension(t *testing.T) {
	t.Parallel()

	result, err := Calculate(Request{
		Jaccard:     0.80,
		ShingleSize: 2,
		ErrorRate:   0.006,
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}

	if result.ShingleSize != 2 {
		t.Fatalf("shingle size = %d, want 2", result.ShingleSize)
	}

	if result.Bands != 36 {
		t.Fatalf("bands = %d, want 36", result.Bands)
	}

	if result.Rows != 9 {
		t.Fatalf("rows = %d, want 9", result.Rows)
	}
}

func TestCalculateRejectsImpossibleRequest(t *testing.T) {
	t.Parallel()

	_, err := Calculate(Request{
		Jaccard:     0.50,
		ShingleSize: 3,
		ErrorRate:   0.000001,
		MaxBands:    2,
		MaxRows:     2,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
