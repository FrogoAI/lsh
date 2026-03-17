package vector

import (
	"testing"

	"github.com/FrogoAI/lsh"
)

func TestCalculateApproximateThreshold(t *testing.T) {
	cases := []struct {
		name  string
		bands int
		rows  int
	}{
		{name: "standard config", bands: 20, rows: 5},
		{name: "high precision", bands: 5, rows: 20},
		{name: "high recall", bands: 50, rows: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Config: lsh.Config{Bands: tc.bands, Rows: tc.rows}}
			threshold := cfg.CalculateApproximateThreshold()

			if threshold < -1.0 || threshold > 1.0 {
				t.Errorf("threshold %f out of cosine range [-1, 1]", threshold)
			}
		})
	}
}

func TestCalculateApproximateThreshold_EdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		bands int
		rows  int
		want  float64
	}{
		{name: "zero bands", bands: 0, rows: 5, want: 0.0},
		{name: "zero rows", bands: 5, rows: 0, want: 0.0},
		{name: "both zero", bands: 0, rows: 0, want: 0.0},
		{name: "negative bands", bands: -1, rows: 5, want: 0.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Config: lsh.Config{Bands: tc.bands, Rows: tc.rows}}
			got := cfg.CalculateApproximateThreshold()

			if got != tc.want {
				t.Errorf("got %f, want %f", got, tc.want)
			}
		})
	}
}

func TestGetConfigFromEnv(t *testing.T) {
	t.Setenv("VLSH_BANDS", "30")
	t.Setenv("VLSH_ROWS", "4")
	t.Setenv("VLSH_VECTOR_DIMENSIONS", "10")
	t.Setenv("VLSH_COS_THRESHOLD", "0.8")
	t.Setenv("VLSH_MAX_BUCKET_SIZE", "300")
	t.Setenv("VLSH_MAX_TOTAL_CANDIDATES", "150")
	t.Setenv("VLSH_SEED", "99")

	cfg, err := GetConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Bands != 30 {
		t.Errorf("Bands: got %d, want 30", cfg.Bands)
	}

	if cfg.VectorDimensions != 10 {
		t.Errorf("VectorDimensions: got %d, want 10", cfg.VectorDimensions)
	}

	if cfg.CosineThreshold != 0.8 {
		t.Errorf("CosineThreshold: got %f, want 0.8", cfg.CosineThreshold)
	}
}

func TestHashVersion_Deterministic(t *testing.T) {
	cfg := &Config{
		Config: lsh.Config{
			Bands: 20, Rows: 5,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 42,
		},
		VectorDimensions: 18,
		CosineThreshold:  0.7,
	}

	h1, err := cfg.HashVersion("grp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h2, err := cfg.HashVersion("grp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 != h2 {
		t.Errorf("non-deterministic: %s vs %s", h1, h2)
	}

	cfg2 := *cfg
	cfg2.VectorDimensions = 10

	h3, err := cfg2.HashVersion("grp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 == h3 {
		t.Error("different config produced same hash")
	}
}
