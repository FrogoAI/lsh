package lsh

import (
	"testing"

	"github.com/FrogoAI/testutils"
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
			cfg := &Config{Bands: tc.bands, Rows: tc.rows}
			got := cfg.CalculateApproximateThreshold()

			diff := got - tc.want
			if diff < -0.001 || diff > 0.001 {
				t.Errorf("got %f, want ~%f", got, tc.want)
			}
		})
	}
}

func TestHashVersion_TableDriven(t *testing.T) {
	baseConfig := Config{
		Bands:              20,
		Rows:               5,
		ShingleSize:        3,
		JaccardThreshold:   0.6,
		MaxBucketSize:      200,
		MaxTotalCandidates: 100,
		Seed:               1337,
	}

	baseHash, err := baseConfig.HashVersion("test")
	if err != nil {
		t.Fatalf("Failed to compute base hash: %v", err)
	}

	testcases := []struct {
		name     string
		modifier func(c *Config)
		expected string
	}{
		{
			name:     "Determinism: No Changes",
			modifier: func(_ *Config) {}, // Do nothing
			expected: baseHash,
		},
		{
			name:     "Sensitivity: Change Bands",
			modifier: func(c *Config) { c.Bands = 21 },
			expected: "b0ed184474ac53b8",
		},
		{
			name:     "Sensitivity: Change ShingleSize",
			modifier: func(c *Config) { c.Bands = 2 },
			expected: "49da837d37797634",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			modConfig := baseConfig
			tc.modifier(&modConfig)

			gotHash, err := modConfig.HashVersion("test")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			testutils.Equal(t, gotHash, tc.expected)
		})
	}
}
