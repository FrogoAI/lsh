package memory

import (
	"sync"

	"github.com/FrogoAI/lsh/model"
)

type Repository struct {
	mu       sync.RWMutex
	buckets  map[string][]string
	lens     map[string][]int
	profiles map[string]model.Record
}

func NewRepository() *Repository {
	return &Repository{
		buckets:  make(map[string][]string),
		lens:     make(map[string][]int),
		profiles: make(map[string]model.Record),
	}
}

func (r *Repository) AddToBucket(k string, v string, len int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buckets[k] = append(r.buckets[k], v)
	r.lens[k] = append(r.lens[k], len)
	return nil
}

func (r *Repository) GetBucketMembers(k string) ([]string, []int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if v, ok := r.buckets[k]; ok {
		out := make([]string, len(v))
		copy(out, v)

		outL := make([]int, len(v))
		copy(outL, r.lens[k])

		return out, outL, nil
	}

	return nil, nil, nil
}

func (r *Repository) SaveRecord(u model.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.profiles[u.ID] = u
	return nil
}

func (r *Repository) GetRecords(ids []string) (map[string]model.Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make(map[string]model.Record)
	for _, id := range ids {
		if p, ok := r.profiles[id]; ok {
			res[id] = p
		}
	}

	return res, nil
}

func (r *Repository) Close() {}
