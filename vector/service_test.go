package vector

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/FrogoAI/lsh/v2"
	"github.com/FrogoAI/lsh/v2/repositories"
	"github.com/FrogoAI/lsh/v2/repositories/memory"
)

func vectorConfig(dims int) *Config {
	return &Config{
		Config: lsh.Config{
			Bands: 20, Rows: 5,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 42,
		},
		VectorDimensions: dims,
		CosineThreshold:  0.7,
	}
}

func TestUpsert_Basic(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := NewService(repo, vectorConfig(3))

	id, err := svc.Upsert(ctx, "grp", []float64{1.0, 2.0, 3.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id == "" {
		t.Fatal("expected non-empty id")
	}

	recs, err := repo.GetRecords([]string{id})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
}

func TestUpsert_Validation(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := NewService(repo, vectorConfig(3))

	cases := []struct {
		name    string
		vec     []float64
		wantErr error
	}{
		{name: "empty vector", vec: []float64{}, wantErr: ErrEmptyVector},
		{name: "nil vector", vec: nil, wantErr: ErrEmptyVector},
		{name: "too few dimensions", vec: []float64{1.0, 2.0}, wantErr: ErrWrongDimension},
		{name: "too many dimensions", vec: []float64{1.0, 2.0, 3.0, 4.0}, wantErr: ErrWrongDimension},
		{name: "single element", vec: []float64{1.0}, wantErr: ErrWrongDimension},
		{name: "zero vector succeeds", vec: []float64{0.0, 0.0, 0.0}, wantErr: nil},
		{name: "negative values succeed", vec: []float64{-1.0, -2.0, -3.0}, wantErr: nil},
		{name: "very large values succeed", vec: []float64{1e18, 1e18, 1e18}, wantErr: nil},
		{name: "very small values succeed", vec: []float64{1e-18, 1e-18, 1e-18}, wantErr: nil},
		{name: "mixed sign values succeed", vec: []float64{-100.0, 0.0, 100.0}, wantErr: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Upsert(ctx, "grp", tc.vec)
			if err != tc.wantErr {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestUpsert_BehavioralScenarios(t *testing.T) {
	ctx := context.Background()

	t.Run("bonus hunting pattern clusters together", func(t *testing.T) {
		repo := memory.NewRepository()
		cfg := vectorConfig(5)
		cfg.CosineThreshold = 0.9
		svc := NewService(repo, cfg)

		// Two bonus hunters: low deposits, single withdrawal, high ratio
		hunter1 := []float64{2.0, 1.0, 0.95, 1.1, 1.0} // deposit_count, withdrawal_count, wd_ratio, bet_ratio, promo
		hunter2 := []float64{3.0, 1.0, 0.98, 1.15, 1.0}

		id1, err := svc.Upsert(ctx, "org1", hunter1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		id2, err := svc.Upsert(ctx, "org1", hunter2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if id1 != id2 {
			t.Errorf("similar bonus hunters should share behavioural_id, got %s vs %s", id1, id2)
		}
	})

	t.Run("different behaviors get different IDs", func(t *testing.T) {
		repo := memory.NewRepository()
		cfg := vectorConfig(5)
		cfg.CosineThreshold = 0.9
		svc := NewService(repo, cfg)

		// Normal user: many deposits, moderate activity
		normal := []float64{20.0, 5.0, 0.3, 8.0, 0.0}
		// Laundering pattern: equal deposits/withdrawals, ratio near 1
		launderer := []float64{5.0, 5.0, 0.98, 0.1, 0.0}

		id1, err := svc.Upsert(ctx, "org1", normal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		id2, err := svc.Upsert(ctx, "org1", launderer)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if id1 == id2 {
			t.Error("normal user and launderer should get different behavioural_ids")
		}
	})
}

func TestUpsert_CloseVectorsReturnSameID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(4)
	cfg.CosineThreshold = 0.5
	svc := NewService(repo, cfg)

	v1 := []float64{1.0, 2.0, 3.0, 4.0}
	v2 := []float64{1.1, 2.1, 3.1, 4.1}

	id1, err := svc.Upsert(ctx, "grp", v1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	id2, err := svc.Upsert(ctx, "grp", v2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected same id for close vectors, got %s vs %s", id1, id2)
	}
}

func TestUpsert_FarVectorsReturnDifferentID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(4)
	cfg.CosineThreshold = 0.9
	svc := NewService(repo, cfg)

	id1, err := svc.Upsert(ctx, "grp", []float64{1.0, 0.0, 0.0, 0.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	id2, err := svc.Upsert(ctx, "grp", []float64{0.0, 0.0, 0.0, 1.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id1 == id2 {
		t.Error("expected different ids for orthogonal vectors")
	}
}

func TestUpsert_DeterministicID(t *testing.T) {
	ctx := context.Background()
	repo := newSpyRepository()
	svc := NewService(repo, vectorConfig(3))

	vec := []float64{1.0, 2.0, 3.0}

	id1, err := svc.Upsert(ctx, "grp", vec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if atomic.LoadInt64(&repo.saveCalls) != 1 {
		t.Error("expected 1 save call")
	}

	id2, err := svc.Upsert(ctx, "grp", vec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected same id, got %s vs %s", id1, id2)
	}

	if atomic.LoadInt64(&repo.saveCalls) != 1 {
		t.Error("expected save not called again for identical vector")
	}
}

func TestUpsert_ResolvedCache(t *testing.T) {
	ctx := context.Background()
	repo := newSpyRepository()
	cfg := vectorConfig(4)
	cfg.CosineThreshold = 0.5
	svc := NewService(repo, cfg)

	v1 := []float64{1.0, 2.0, 3.0, 4.0}
	v2 := []float64{1.1, 2.1, 3.1, 4.1}

	originalID, err := svc.Upsert(ctx, "grp", v1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	batchCallsAfterInsert := atomic.LoadInt64(&repo.batchGetCalls)

	// First similar call: full LSH
	id, err := svc.Upsert(ctx, "grp", v2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != originalID {
		t.Errorf("expected %s, got %s", originalID, id)
	}

	// Second similar call: cache hit
	id2, err := svc.Upsert(ctx, "grp", v2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id2 != originalID {
		t.Errorf("expected %s, got %s", originalID, id2)
	}

	totalBatchGets := atomic.LoadInt64(&repo.batchGetCalls)
	expectedBatchGets := batchCallsAfterInsert + 1

	if totalBatchGets != expectedBatchGets {
		t.Errorf("expected %d batch gets, got %d", expectedBatchGets, totalBatchGets)
	}
}

func TestUpsert_L2CacheSurvivesPodRestart(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(4)
	cfg.CosineThreshold = 0.5

	svc1 := NewService(repo, cfg)

	v1 := []float64{1.0, 2.0, 3.0, 4.0}
	v2 := []float64{1.1, 2.1, 3.1, 4.1}

	originalID, err := svc1.Upsert(ctx, "grp", v1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc1.Upsert(ctx, "grp", v2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate pod restart: new service instance, same repo
	svc2 := NewService(repo, cfg)

	id, err := svc2.Upsert(ctx, "grp", v2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != originalID {
		t.Errorf("expected %s from L2 cache, got %s", originalID, id)
	}
}

func TestUpsert_ManyUniqueVectors(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(3)
	cfg.Bands = 5
	cfg.Rows = 2
	svc := NewService(repo, cfg)

	for i := 0; i < 50; i++ {
		_, err := svc.Upsert(ctx, "grp", []float64{float64(i * 100), float64(i*100 + 1), float64(i*100 + 2)})
		if err != nil {
			t.Fatalf("unexpected error at i=%d: %v", i, err)
		}
	}

	// Should not error with many representatives per bucket
	_, err := svc.Upsert(ctx, "grp", []float64{99.0, 100.0, 101.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetNewID(t *testing.T) {
	svc := NewService(memory.NewRepository(), vectorConfig(3))

	id1 := svc.GetNewID([]float64{1.0, 2.0, 3.0})
	id2 := svc.GetNewID([]float64{1.0, 2.0, 3.0})
	id3 := svc.GetNewID([]float64{4.0, 5.0, 6.0})

	if id1 != id2 {
		t.Error("expected deterministic ID")
	}

	if id1 == id3 {
		t.Error("expected different IDs for different vectors")
	}

	if id1 == "" {
		t.Error("expected non-empty ID")
	}
}

func BenchmarkUpsert(b *testing.B) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(18)
	svc := NewService(repo, cfg)

	vec := make([]float64, 18)
	for i := range vec {
		vec[i] = float64(i) * 0.1
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vec[0] = float64(i)
		_, _ = svc.Upsert(ctx, "grp", vec)
	}
}

// spyRepository wraps memory.Repository for call counting.
type spyRepository struct {
	*memory.Repository
	saveCalls     int64
	batchSetCalls int64
	batchGetCalls int64
}

func newSpyRepository() *spyRepository {
	return &spyRepository{Repository: memory.NewRepository()}
}

func (s *spyRepository) SaveRecord(key string, bins map[string]any) error {
	atomic.AddInt64(&s.saveCalls, 1)

	return s.Repository.SaveRecord(key, bins)
}

func (s *spyRepository) BatchSetRepresentative(keys []string, memberID string, metadata int64) error {
	atomic.AddInt64(&s.batchSetCalls, 1)

	return s.Repository.BatchSetRepresentative(keys, memberID, metadata)
}

func (s *spyRepository) BatchGetRepresentatives(keys []string) (map[string][]repositories.Representative, error) {
	atomic.AddInt64(&s.batchGetCalls, 1)

	return s.Repository.BatchGetRepresentatives(keys)
}
