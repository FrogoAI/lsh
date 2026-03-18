package dedup

import (
	"testing"

	"github.com/FrogoAI/lsh/v2"
)

func TestCalculateApproximateThreshold(t *testing.T) {
	cases := []struct {
		name  string
		bands int
		rows  int
		want  float64
	}{
		{name: "standard config", bands: 20, rows: 5, want: 0.5493},
		{name: "single band single row", bands: 1, rows: 1, want: 1.0},
		{name: "zero bands", bands: 0, rows: 5, want: 0.0},
		{name: "zero rows", bands: 5, rows: 0, want: 0.0},
		{name: "both zero", bands: 0, rows: 0, want: 0.0},
		{name: "negative bands", bands: -1, rows: 5, want: 0.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Config: lsh.Config{Bands: tc.bands, Rows: tc.rows}}
			got := cfg.CalculateApproximateThreshold()

			diff := got - tc.want
			if diff < -0.001 || diff > 0.001 {
				t.Errorf("got %f, want ~%f", got, tc.want)
			}
		})
	}
}

func TestHashVersion_Deterministic(t *testing.T) {
	cfg := &Config{
		Config: lsh.Config{
			Bands: 20, Rows: 5,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 1337,
		},
		ShingleSize:      3,
		JaccardThreshold: 0.6,
	}

	h1, err := cfg.HashVersion("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h2, err := cfg.HashVersion("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 != h2 {
		t.Errorf("non-deterministic: %s != %s", h1, h2)
	}

	cfg2 := *cfg
	cfg2.Bands = 21

	h3, err := cfg2.HashVersion("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 == h3 {
		t.Error("different config produced same hash")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		bands   int
		rows    int
		thresh  float64
		wantErr bool
	}{
		{name: "B40_R5_jac0.6 (default)", bands: 40, rows: 5, thresh: 0.6, wantErr: false},
		{name: "B40_R5_jac0.8", bands: 40, rows: 5, thresh: 0.8, wantErr: false},
		{name: "B20_R5_jac0.6", bands: 20, rows: 5, thresh: 0.6, wantErr: false},
		{name: "B40_R8_jac0.6 (approx=0.644 > 0.6)", bands: 40, rows: 8, thresh: 0.6, wantErr: true},
		{name: "B40_R8_jac0.7 (approx=0.644 < 0.7)", bands: 40, rows: 8, thresh: 0.7, wantErr: false},
		{name: "B10_R10_jac0.5 (approx=0.794 > 0.5)", bands: 10, rows: 10, thresh: 0.5, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Config:           lsh.Config{Bands: tc.bands, Rows: tc.rows},
				JaccardThreshold: tc.thresh,
				ShingleSize:      3,
			}

			err := cfg.Validate()

			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil (approx=%.3f, thresh=%.3f)",
					cfg.CalculateApproximateThreshold(), tc.thresh)
			}

			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
