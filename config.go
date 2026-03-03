package lsh

import (
	"encoding/hex"
	"math"
	"strings"

	"github.com/caarlos0/env/v10"
	"github.com/cespare/xxhash/v2"

	"github.com/FrogoAI/packer"
)

const (
	EnvPrefix = "LSH"
)

/*
Config defines the parameters for the LSH (Locality-Sensitive Hashing) pipeline.

- Bands and Rows control how MinHash signatures are split into band keys:

  - signature size = Bands * Rows

  - increasing Bands (with fixed Rows) generally increases recall: more chances for
    similar items to land in at least one common bucket.

  - increasing Rows (with fixed Bands) generally increases precision: a stricter
    requirement to match within a band.

    Together they define an *approximate* similarity level where collisions become likely
    (often estimated as s ≈ (1/Bands)^(1/Rows)). This is a probabilistic candidate generator.

  - JaccardThreshold is the final similarity filter used after candidate generation.
    We intentionally keep the LSH bucketing stage “looser” (i.e., allowing candidates at a
    lower similarity) so that *similar* items are saved/located via similar buckets.
    Then JaccardThreshold decides whether a candidate is “similar enough” to return the
    same ID or should be treated as different (new) ID.
*/
type Config struct {
	Bands              int     `env:"_BANDS" envDefault:"40"`
	Rows               int     `env:"_ROWS" envDefault:"5"`
	ShingleSize        int     `env:"_SHINGLE_SIZE" envDefault:"3"`
	JaccardThreshold   float64 `env:"_JAC_THRESHOLD" envDefault:"0.6"`
	MaxBucketSize      int     `env:"_MAX_BUCKET_SIZE" envDefault:"200"`
	MaxTotalCandidates int     `env:"_MAX_TOTAL_CANDIDATES" envDefault:"100"`
	Seed               int64   `env:"_SEED" envDefault:"13374269"`
}

func GetLSHConfigFromEnv() (*Config, error) {
	c := new(Config)

	err := env.ParseWithOptions(c, env.Options{
		Prefix: strings.ToUpper(EnvPrefix),
	})
	if err != nil {
		return nil, err
	}

	return c, err
}

// CalculateApproximateThreshold computes the approximate Jaccard similarity threshold
// at which the LSH configuration (Bands and Rows) is most sensitive.
// This is the point where the probability of two items being hashed to the same
// bucket begins to rise sharply. The formula is s ≈ (1/B)^(1/R).
func (c *Config) CalculateApproximateThreshold() float64 {
	if c.Bands <= 0 || c.Rows <= 0 {
		return 0.0
	}

	return math.Pow(1.0/float64(c.Bands), 1.0/float64(c.Rows))
}

func (c *Config) HashVersion(group string) (string, error) {
	enc := packer.NewBinaryEncoder()

	err := enc.Encode(group)
	if err != nil {
		return "", err
	}

	err = enc.Encode(c.Bands)
	if err != nil {
		return "", err
	}

	err = enc.Encode(c.Rows)
	if err != nil {
		return "", err
	}

	err = enc.Encode(c.ShingleSize)
	if err != nil {
		return "", err
	}

	err = enc.Encode(c.JaccardThreshold)
	if err != nil {
		return "", err
	}

	err = enc.Encode(c.MaxBucketSize)
	if err != nil {
		return "", err
	}

	err = enc.Encode(c.MaxTotalCandidates)
	if err != nil {
		return "", err
	}

	err = enc.Encode(c.Seed)
	if err != nil {
		return "", err
	}

	s := xxhash.New()

	_, err = s.Write(enc.Bytes())
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(s.Sum(nil)), nil
}
