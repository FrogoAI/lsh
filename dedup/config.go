package dedup

import (
	"fmt"
	"math"
	"strings"

	"github.com/caarlos0/env/v10"

	"github.com/FrogoAI/lsh/v2"
)

const EnvPrefix = "LSH"

type Config struct {
	lsh.Config
	ShingleSize      int     `env:"_SHINGLE_SIZE" envDefault:"3"`
	JaccardThreshold float64 `env:"_JAC_THRESHOLD" envDefault:"0.6"`
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

// CalculateApproximateThreshold computes s ~ (1/B)^(1/R) —
// the Jaccard similarity where the LSH collision probability transitions sharply.
func (c *Config) CalculateApproximateThreshold() float64 {
	if c.Bands <= 0 || c.Rows <= 0 {
		return 0.0
	}

	return math.Pow(1.0/float64(c.Bands), 1.0/float64(c.Rows))
}

// Validate checks that the LSH approximate threshold is strictly below JaccardThreshold.
// If violated, recall at the application threshold drops below 50%.
func (c *Config) Validate() error {
	approx := c.CalculateApproximateThreshold()

	if approx >= c.JaccardThreshold {
		return fmt.Errorf(
			"%w: LSH approximate threshold (%.3f) must be below JaccardThreshold (%.3f), increase Bands or decrease Rows",
			lsh.ErrInvalidConfig, approx, c.JaccardThreshold,
		)
	}

	return nil
}

// HashVersion computes a deterministic prefix from group + all config fields.
func (c *Config) HashVersion(group string) (string, error) {
	return lsh.HashVersion(group,
		c.Bands, c.Rows, c.ShingleSize, c.JaccardThreshold,
		c.MaxBucketSize, c.MaxTotalCandidates, c.Seed,
	)
}
