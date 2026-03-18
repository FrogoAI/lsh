package memory

import (
	"sync"

	"github.com/FrogoAI/lsh/v2/repositories"
)

type Repository struct {
	mu      sync.RWMutex
	buckets map[string]map[string]int64 // bucketKey -> memberID -> metadata
	records map[string]map[string]any
	values  map[string]string
}

func NewRepository() *Repository {
	return &Repository{
		buckets: make(map[string]map[string]int64),
		records: make(map[string]map[string]any),
		values:  make(map[string]string),
	}
}

func (r *Repository) SetRepresentative(bucketKey, memberID string, metadata int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.buckets[bucketKey]
	if !ok {
		m = make(map[string]int64)
		r.buckets[bucketKey] = m
	}

	m[memberID] = metadata

	return nil
}

func (r *Repository) BatchSetRepresentative(bucketKeys []string, memberID string, metadata int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, k := range bucketKeys {
		m, ok := r.buckets[k]
		if !ok {
			m = make(map[string]int64)
			r.buckets[k] = m
		}

		m[memberID] = metadata
	}

	return nil
}

func (r *Repository) GetRepresentatives(bucketKey string) ([]repositories.Representative, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.buckets[bucketKey]
	if !ok {
		return nil, nil
	}

	reps := make([]repositories.Representative, 0, len(m))
	for id, meta := range m {
		reps = append(reps, repositories.Representative{ID: id, Metadata: meta})
	}

	return reps, nil
}

func (r *Repository) BatchGetRepresentatives(bucketKeys []string) (map[string][]repositories.Representative, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]repositories.Representative, len(bucketKeys))

	for _, k := range bucketKeys {
		m := r.buckets[k]

		reps := make([]repositories.Representative, 0, len(m))
		for id, meta := range m {
			reps = append(reps, repositories.Representative{ID: id, Metadata: meta})
		}

		result[k] = reps
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
