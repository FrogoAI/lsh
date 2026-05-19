package dedup

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/FrogoAI/lsh/v2/dedup/lshcalc"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/FrogoAI/lsh/v2"
	"github.com/FrogoAI/lsh/v2/repositories"
	"github.com/FrogoAI/lsh/v2/repositories/memory"
)

func defaultConfig() *Config {
	return &Config{
		Config: lsh.Config{
			Bands: 40, Rows: 5,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 13374269,
		},
		ShingleSize:      3,
		JaccardThreshold: 0.6,
	}
}

func TestUpsert90(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()

	res, err := lshcalc.Calculate(lshcalc.Request{
		Jaccard:     0.8,
		ShingleSize: 2,
		ErrorRate:   0.001,
	})
	if err != nil {
		t.Fatalf("lshcalc.Calculate: %v", err)
	}

	svc, err := NewService(repo, &Config{
		Config: lsh.Config{
			Bands: res.Bands, Rows: res.Rows,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 13374269,
		},
		ShingleSize:      res.ShingleSize,
		JaccardThreshold: 0.8,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id, err := svc.Upsert(ctx, "email", "abcdefghijkl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	id2, err := svc.Upsert(ctx, "email", "abcdefghijklm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fmt.Println(id, id2)
}

func TestUpsertEmail(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()

	svc, err := NewService(repo, defaultConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id, err := svc.Upsert(ctx, "email", "maxim@weavers.team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	id2, err := svc.Upsert(ctx, "email", "maxim@weavets.team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != id2 {
		t.Errorf("expected same id for similar input, got %s vs %s", id, id2)
	}

	id3, err := svc.Upsert(ctx, "email", "maxim@weavers.teams")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != id3 {
		t.Errorf("expected same id, got %s vs %s", id, id3)
	}
}

func TestUpsert(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()

	svc, err := NewService(repo, defaultConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id1, err := svc.Upsert(ctx, "users", "John Doe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id1 == "" {
		t.Fatal("expected non-empty id")
	}

	id2, err := svc.Upsert(ctx, "users", "Johnn Doe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected same id for similar input, got %s vs %s", id1, id2)
	}

	id3, err := svc.Upsert(ctx, "users", "Jane Doe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id1 == id3 {
		t.Error("expected different id for dissimilar input")
	}

	for i := 0; i < 1000; i++ {
		_, err := svc.Upsert(ctx, "users", "test"+strconv.Itoa(i)+"@gmail.com")
		if err != nil {
			t.Fatalf("unexpected error at iteration %d: %v", i, err)
		}
	}
}

func TestUpsert_EdgeCases(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "empty input", input: "", wantErr: ErrEmptyInputString},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewRepository()
			cfg := &Config{
				Config: lsh.Config{
					Bands: 20, Rows: 5,
					MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 42,
				},
				ShingleSize:      5,
				JaccardThreshold: 0.6,
			}

			svc, err := NewService(repo, cfg)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			_, err = svc.Upsert(ctx, "g", tc.input)
			if err != tc.wantErr {
				t.Errorf("got err %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestUpsert_ManyUniqueInputs(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := &Config{
		Config: lsh.Config{
			Bands: 5, Rows: 2,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 42,
		},
		ShingleSize:      3,
		JaccardThreshold: 0.6,
	}

	svc, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	for i := 0; i < 50; i++ {
		_, err := svc.Upsert(ctx, "grp", "user"+strconv.Itoa(i)+"@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Should not error with many representatives per bucket
	id, err := svc.Upsert(ctx, "grp", "completely_different_string_xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id == "" {
		t.Fatal("expected non-empty id")
	}
}

func TestUpsert_ResolvedCache(t *testing.T) {
	ctx := context.Background()
	repo := newSpyRepository()

	svc, err := NewService(repo, defaultConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	originalID, err := svc.Upsert(ctx, "email", "maxim@weavers.team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	batchCallsAfterInsert := atomic.LoadInt64(&repo.batchGetCalls)

	// First similar call: full LSH
	id, err := svc.Upsert(ctx, "email", "maxim@weavets.team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != originalID {
		t.Errorf("expected %s, got %s", originalID, id)
	}

	// Second similar call: cache hit
	id2, err := svc.Upsert(ctx, "email", "maxim@weavets.team")
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

func TestUpsert_DeterministicID(t *testing.T) {
	ctx := context.Background()
	repo := newSpyRepository()

	svc, err := NewService(repo, defaultConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id1, err := svc.Upsert(ctx, "grp", "optimization_test_input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if atomic.LoadInt64(&repo.saveCalls) != 1 {
		t.Error("expected 1 save call")
	}

	id2, err := svc.Upsert(ctx, "grp", "optimization_test_input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected same id, got %s vs %s", id1, id2)
	}

	if atomic.LoadInt64(&repo.saveCalls) != 1 {
		t.Error("expected save not called again for identical input")
	}
}

func BenchmarkUpsert(b *testing.B) {
	ctx := context.Background()
	cfg := &Config{
		Config: lsh.Config{
			Bands: 20, Rows: 5,
			MaxBucketSize: 100, MaxTotalCandidates: 100, Seed: 42,
		},
		ShingleSize:      3,
		JaccardThreshold: 0.6,
	}

	b.Run("NewRecord", func(b *testing.B) {
		repo := memory.NewRepository()

		svc, err := NewService(repo, cfg)
		if err != nil {
			b.Fatalf("NewService: %v", err)
		}

		// Seed some data so buckets aren't empty
		for i := 0; i < 100; i++ {
			_, _ = svc.Upsert(ctx, "users", "seed"+strconv.Itoa(i)+"@gmail.com")
		}

		// Use an offset so every iteration produces a truly novel input
		offset := 100000

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			input := fmt.Sprintf("bench_new_%d@example.com", offset+i)
			_, _ = svc.Upsert(ctx, "users", input)
		}
	})

	b.Run("DuplicateMatch", func(b *testing.B) {
		repo := memory.NewRepository()

		svc, err := NewService(repo, cfg)
		if err != nil {
			b.Fatalf("NewService: %v", err)
		}

		target := "duplicate@example.com"
		_, _ = svc.Upsert(ctx, "users", target)

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, _ = svc.Upsert(ctx, "users", target)
		}
	})

	b.Run("SimilarMatch", func(b *testing.B) {
		repo := memory.NewRepository()

		svc, err := NewService(repo, cfg)
		if err != nil {
			b.Fatalf("NewService: %v", err)
		}

		_, _ = svc.Upsert(ctx, "users", "similar_original@example.com")
		// First call resolves and caches; subsequent calls hit L2
		_, _ = svc.Upsert(ctx, "users", "similar_originak@example.com")

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, _ = svc.Upsert(ctx, "users", "similar_originak@example.com")
		}
	})
}

func TestUpsert_L2CacheSurvivesPodRestart(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := defaultConfig()

	svc1, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	originalID, err := svc1.Upsert(ctx, "email", "maxim@weavers.team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc1.Upsert(ctx, "email", "maxim@weavets.team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate pod restart: new service instance, same repo
	svc2, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id, err := svc2.Upsert(ctx, "email", "maxim@weavets.team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != originalID {
		t.Errorf("expected %s from L2 cache, got %s", originalID, id)
	}
}

func TestUpsert_MaxTotalCandidatesCap(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := &Config{
		Config: lsh.Config{
			Bands: 40, Rows: 5,
			MaxBucketSize: 200, MaxTotalCandidates: 2, Seed: 42,
		},
		ShingleSize:      3,
		JaccardThreshold: 0.6,
	}

	svc, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	for i := 0; i < 20; i++ {
		_, err := svc.Upsert(ctx, "grp", "candidate"+strconv.Itoa(i)+"@test.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Should not panic or error with many candidates
	_, err = svc.Upsert(ctx, "grp", "candidate99@test.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetNewID(t *testing.T) {
	svc, err := NewService(memory.NewRepository(), defaultConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id1 := svc.GetNewID("hello")
	id2 := svc.GetNewID("hello")
	id3 := svc.GetNewID("world")

	if id1 != id2 {
		t.Error("expected deterministic ID")
	}

	if id1 == id3 {
		t.Error("expected different IDs for different inputs")
	}

	if id1 == "" {
		t.Error("expected non-empty ID")
	}
}

func TestGetConfigFromEnv(t *testing.T) {
	t.Setenv("LSH_BANDS", "30")
	t.Setenv("LSH_ROWS", "4")
	t.Setenv("LSH_SHINGLE_SIZE", "5")
	t.Setenv("LSH_JAC_THRESHOLD", "0.8")
	t.Setenv("LSH_MAX_BUCKET_SIZE", "300")
	t.Setenv("LSH_MAX_TOTAL_CANDIDATES", "150")
	t.Setenv("LSH_SEED", "99")

	cfg, err := GetConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Bands != 30 {
		t.Errorf("Bands: got %d, want 30", cfg.Bands)
	}

	if cfg.Rows != 4 {
		t.Errorf("Rows: got %d, want 4", cfg.Rows)
	}

	if cfg.ShingleSize != 5 {
		t.Errorf("ShingleSize: got %d, want 5", cfg.ShingleSize)
	}

	if cfg.JaccardThreshold != 0.8 {
		t.Errorf("JaccardThreshold: got %f, want 0.8", cfg.JaccardThreshold)
	}
}

func TestRecordFromBins_Branches(t *testing.T) {
	cases := []struct {
		name   string
		bins   map[string]any
		wantOK bool
		wantID string
	}{
		{
			name:   "valid with []uint64 signature",
			bins:   map[string]any{"i": "hello", "g": "grp", "s": []uint64{1, 2, 3}},
			wantOK: true,
			wantID: "k1",
		},
		{
			name: "valid with []any signature (int types)",
			bins: map[string]any{
				"i": "hello", "g": "grp",
				"s": []any{int(1), int64(2), float64(3)},
			},
			wantOK: true,
			wantID: "k2",
		},
		{
			name:   "missing input returns false",
			bins:   map[string]any{"g": "grp", "s": []uint64{1}},
			wantOK: false,
		},
		{
			name:   "empty bins",
			bins:   map[string]any{},
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := recordFromBins(repositories.Record{Key: tc.wantID, Bins: tc.bins})
			if ok != tc.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tc.wantOK)
			}

			if tc.wantOK && rec.ID != tc.wantID {
				t.Errorf("ID: got %s, want %s", rec.ID, tc.wantID)
			}
		})
	}
}

func TestUpsert_WithMetrics(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()

	svc, err := NewService(repo, defaultConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	m := noop.NewMeterProvider().Meter("test")

	inst, err := lsh.NewInstruments(m, "lsh.dedup.")
	if err != nil {
		t.Fatalf("NewInstruments: %v", err)
	}

	svc.WithMetrics(inst)

	// Novel -> ResultNew
	_, err = svc.Upsert(ctx, "grp", "maxim@weavers.team")
	if err != nil {
		t.Fatalf("novel: %v", err)
	}

	// Exact duplicate -> ResultL1Hit
	_, err = svc.Upsert(ctx, "grp", "maxim@weavers.team")
	if err != nil {
		t.Fatalf("l1 hit: %v", err)
	}

	// Similar -> ResultMatch
	_, err = svc.Upsert(ctx, "grp", "maxim@weavets.team")
	if err != nil {
		t.Fatalf("match: %v", err)
	}

	// Same similar again -> ResultL2Hit
	_, err = svc.Upsert(ctx, "grp", "maxim@weavets.team")
	if err != nil {
		t.Fatalf("l2 hit: %v", err)
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
