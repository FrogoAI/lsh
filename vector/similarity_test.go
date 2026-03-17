package vector

import (
	"math"
	"testing"
)

func TestEstimateCosine(t *testing.T) {
	cases := []struct {
		name string
		sig1 []uint64
		sig2 []uint64
		want float64
		tol  float64
	}{
		{name: "identical", sig1: []uint64{1, 1, 0, 1}, sig2: []uint64{1, 1, 0, 1}, want: 1.0, tol: 0.01},
		{name: "opposite", sig1: []uint64{1, 1, 1, 1}, sig2: []uint64{0, 0, 0, 0}, want: -1.0, tol: 0.01},
		{name: "half match", sig1: []uint64{1, 1, 0, 0}, sig2: []uint64{1, 1, 1, 1}, want: 0.0, tol: 0.01},
		{name: "empty", sig1: []uint64{}, sig2: []uint64{}, want: 0.0, tol: 0.01},
		{name: "different lengths", sig1: []uint64{1}, sig2: []uint64{1, 0}, want: 0.0, tol: 0.01},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateCosine(tc.sig1, tc.sig2)
			if math.Abs(got-tc.want) > tc.tol {
				t.Errorf("got %f, want ~%f", got, tc.want)
			}
		})
	}
}

func TestExactCosine(t *testing.T) {
	cases := []struct {
		name string
		v1   []float64
		v2   []float64
		want float64
		tol  float64
	}{
		{name: "parallel", v1: []float64{1, 0}, v2: []float64{2, 0}, want: 1.0, tol: 0.001},
		{name: "perpendicular", v1: []float64{1, 0}, v2: []float64{0, 1}, want: 0.0, tol: 0.001},
		{name: "opposite", v1: []float64{1, 0}, v2: []float64{-1, 0}, want: -1.0, tol: 0.001},
		{name: "45 degrees", v1: []float64{1, 0}, v2: []float64{1, 1}, want: 0.7071, tol: 0.001},
		{name: "zero vector", v1: []float64{0, 0}, v2: []float64{1, 0}, want: 0.0, tol: 0.001},
		{name: "empty", v1: []float64{}, v2: []float64{}, want: 0.0, tol: 0.001},
		{name: "different lengths", v1: []float64{1}, v2: []float64{1, 2}, want: 0.0, tol: 0.001},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExactCosine(tc.v1, tc.v2)
			if math.Abs(got-tc.want) > tc.tol {
				t.Errorf("got %f, want ~%f", got, tc.want)
			}
		})
	}
}
