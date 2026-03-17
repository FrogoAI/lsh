package repositories

// BucketMember represents a single member in a bucket with opaque metadata.
// For string dedup, Metadata = shingle length. For vectors, Metadata = timestamp.
type BucketMember struct {
	ID       string
	Metadata int64
}

// Record is an opaque key-value record. The caller defines bin names and handles serialization.
type Record struct {
	Key  string
	Bins map[string]any
}

// Storage is a generic persistence interface with no domain knowledge.
// It stores buckets (maps of members) and opaque records (key-value bins).
type Storage interface {
	// Bucket operations
	AddBucketMember(bucketKey, memberID string, metadata int64) error
	BatchAddBucketMember(bucketKeys []string, memberID string, metadata int64) error
	GetBucketMembers(bucketKey string) ([]BucketMember, error)
	BatchGetBucketMembers(bucketKeys []string) (map[string][]BucketMember, error)

	// Record operations (opaque key-value)
	SaveRecord(key string, bins map[string]any) error
	GetRecords(keys []string) ([]Record, error)

	// Key-value pairs (for caches, resolved IDs, etc.)
	PutValue(key, value string) error
	GetValue(key string) (string, error)

	Close()
}
