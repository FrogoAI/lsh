package vector

import (
	"fmt"
	"math"
	"strings"

	"github.com/caarlos0/env/v10"

	"github.com/FrogoAI/lsh/v2"
)

const EnvPrefix = "VLSH"

type Config struct {
	lsh.Config
	VectorDimensions int     `env:"_VECTOR_DIMENSIONS" envDefault:"20"`
	CosineThreshold  float64 `env:"_COS_THRESHOLD" envDefault:"0.7"`
}

func GetConfigFromEnv() (*Config, error) {
	c := new(Config)

	err := env.ParseWithOptions(c, env.Options{
		Prefix: strings.ToUpper(EnvPrefix),
	})
	if err != nil {
		return nil, err
	}

	return c, nil
}

// CalculateApproximateThreshold returns the cosine similarity where the LSH
// collision probability transitions sharply for the given Bands/Rows config.
//
//	P(one band) = (1 - arccos(s)/pi)^R
//	P(any band) = 1 - (1 - P(one band))^B
//
// We find s where P(any band) ~ 0.5 using binary search.
func (c *Config) CalculateApproximateThreshold() float64 {
	if c.Bands <= 0 || c.Rows <= 0 {
		return 0.0
	}

	const half = 0.5

	lo, hi := -1.0, 1.0

	for range 100 { //nolint:mnd
		mid := (lo + hi) * half
		pOneBand := math.Pow(1.0-math.Acos(mid)/math.Pi, float64(c.Rows))
		pAnyBand := 1.0 - math.Pow(1.0-pOneBand, float64(c.Bands))

		if pAnyBand < half {
			lo = mid
		} else {
			hi = mid
		}
	}

	return (lo + hi) * half
}

// Validate checks that the LSH approximate threshold is strictly below CosineThreshold.
// If violated, recall at the application threshold drops below 50% — the system misses
// more matches than it finds. This catches dangerous B/R configurations early.
func (c *Config) Validate() error {
	approx := c.CalculateApproximateThreshold()

	if approx >= c.CosineThreshold {
		return fmt.Errorf(
			"%w: LSH approximate threshold (%.3f) must be below CosineThreshold (%.3f), increase Bands or decrease Rows",
			lsh.ErrInvalidConfig, approx, c.CosineThreshold,
		)
	}

	return nil
}

// HashVersion computes a deterministic prefix from group + all config fields.
func (c *Config) HashVersion(group string) (string, error) {
	return lsh.HashVersion(group,
		c.Bands, c.Rows, c.VectorDimensions, c.CosineThreshold,
		c.MaxBucketSize, c.MaxTotalCandidates, c.Seed,
	)
}
