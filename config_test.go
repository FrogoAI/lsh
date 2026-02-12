package lsh

import (
	"testing"

	"github.com/FrogoAI/testutils"
)

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
