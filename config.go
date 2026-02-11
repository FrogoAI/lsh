package lsh

import (
	"encoding/hex"
	"strings"

	"github.com/FrogoAI/packer"
	"github.com/caarlos0/env/v10"
	"github.com/cespare/xxhash/v2"
)

const (
	SignatureSize int = 100

	EnvPrefix = "LSH"
)

type Config struct {
	Bands              int     `env:"_BANDS" envDefault:"20"`
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
