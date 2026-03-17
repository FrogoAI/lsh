package lsh

// Config holds universal LSH parameters shared across all use cases.
type Config struct {
	Bands              int   `env:"_BANDS" envDefault:"40"`
	Rows               int   `env:"_ROWS" envDefault:"5"`
	MaxBucketSize      int   `env:"_MAX_BUCKET_SIZE" envDefault:"200"`
	MaxTotalCandidates int   `env:"_MAX_TOTAL_CANDIDATES" envDefault:"100"`
	Seed               int64 `env:"_SEED" envDefault:"13374269"`
}

// SignatureSize returns the total number of hash functions (Bands * Rows).
func (c *Config) SignatureSize() int {
	return c.Bands * c.Rows
}
