package repositories

// Representative is a cluster leader stored in a bucket.
// Each bucket maps memberID -> metadata (set semantics, not append).
// For dedup: Metadata = shingle length. For vectors: Metadata = 0.
type Representative struct {
	ID       string
	Metadata int64
}

// Record is an opaque key-value record. The caller defines bin names and handles serialization.
type Record struct {
	Key  string
	Bins map[string]any
}

// Storage is a generic persistence interface with no domain knowledge.
// Buckets store representatives (bounded, set-keyed by memberID).
// Records store full data (vectors, signatures, etc.).
type Storage interface {
	// SetRepresentative upserts a representative into a bucket.
	// If memberID already exists in the bucket, metadata is updated (idempotent).
	SetRepresentative(bucketKey, memberID string, metadata int64) error

	// BatchSetRepresentative upserts a representative into multiple buckets.
	BatchSetRepresentative(bucketKeys []string, memberID string, metadata int64) error

	// GetRepresentatives returns all representatives in a bucket.
	GetRepresentatives(bucketKey string) ([]Representative, error)

	// BatchGetRepresentatives returns representatives for multiple buckets.
	BatchGetRepresentatives(bucketKeys []string) (map[string][]Representative, error)

	// Record operations (opaque key-value)
	SaveRecord(key string, bins map[string]any) error
	GetRecords(keys []string) ([]Record, error)

	// Key-value pairs (for caches, resolved IDs, etc.)
	PutValue(key, value string) error
	GetValue(key string) (string, error)

	Close()
}
