package memory

import (
	"sync"

	"github.com/FrogoAI/lsh/repositories"
)

type Repository struct {
	mu      sync.RWMutex
	buckets map[string][]repositories.BucketMember
	records map[string]map[string]any
	values  map[string]string
}

func NewRepository() *Repository {
	return &Repository{
		buckets: make(map[string][]repositories.BucketMember),
		records: make(map[string]map[string]any),
		values:  make(map[string]string),
	}
}

func (r *Repository) AddBucketMember(bucketKey, memberID string, metadata int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buckets[bucketKey] = append(r.buckets[bucketKey], repositories.BucketMember{
		ID:       memberID,
		Metadata: metadata,
	})

	return nil
}

func (r *Repository) BatchAddBucketMember(bucketKeys []string, memberID string, metadata int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, k := range bucketKeys {
		r.buckets[k] = append(r.buckets[k], repositories.BucketMember{
			ID:       memberID,
			Metadata: metadata,
		})
	}

	return nil
}

func (r *Repository) GetBucketMembers(bucketKey string) ([]repositories.BucketMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	members, ok := r.buckets[bucketKey]
	if !ok {
		return nil, nil
	}

	out := make([]repositories.BucketMember, len(members))
	copy(out, members)

	return out, nil
}

func (r *Repository) BatchGetBucketMembers(bucketKeys []string) (map[string][]repositories.BucketMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]repositories.BucketMember, len(bucketKeys))

	for _, k := range bucketKeys {
		members := r.buckets[k]

		out := make([]repositories.BucketMember, len(members))
		copy(out, members)

		result[k] = out
	}

	return result, nil
}

func (r *Repository) SaveRecord(key string, bins map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := make(map[string]any, len(bins))
	for k, v := range bins {
		cp[k] = v
	}

	r.records[key] = cp

	return nil
}

func (r *Repository) GetRecords(keys []string) ([]repositories.Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []repositories.Record

	for _, key := range keys {
		if bins, ok := r.records[key]; ok {
			result = append(result, repositories.Record{Key: key, Bins: bins})
		}
	}

	return result, nil
}

func (r *Repository) PutValue(key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.values[key] = value

	return nil
}

func (r *Repository) GetValue(key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.values[key], nil
}

func (r *Repository) Close() {}
