package lsh

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	as "github.com/aerospike/aerospike-client-go/v8"
	"github.com/k0kubun/pp/v3"

	"github.com/FrogoAI/lsh/model"
	"github.com/FrogoAI/lsh/repositories/aerospike"
	"github.com/FrogoAI/lsh/repositories/memory"
	"github.com/FrogoAI/testutils"
)

func TestUpsertEmail(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()

	cfg, err := GetLSHConfigFromEnv()
	testutils.Equal(t, err, nil)

	service := NewSimilarityService(repo, cfg)

	id, err := service.Upsert(ctx, "email", "maxim@weavers.team")
	testutils.Equal(t, err, nil)

	pp.Println(id)

	id2, err := service.Upsert(ctx, "email", "maxim@weavets.team")
	testutils.Equal(t, err, nil)
	testutils.Equal(t, id, id2)

	id3, err := service.Upsert(ctx, "email", "maxim@weavers.teams")
	testutils.Equal(t, err, nil)
	testutils.Equal(t, id, id3)

	id4, err := service.Upsert(ctx, "email", "maxem@weavers.teams")
	testutils.Equal(t, err, nil)
	testutils.Equal(t, id, id4)
}

func TestUpsert(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()

	cfg, err := GetLSHConfigFromEnv()
	testutils.Equal(t, err, nil)

	service := NewSimilarityService(repo, cfg)

	id1, err := service.Upsert(ctx, "users", "John Doe")
	testutils.Equal(t, err, nil)

	if id1 == "" {
		t.Fatalf("We must have unempty bucket, got: %s", id1)
	}

	id2, err := service.Upsert(ctx, "users", "Johnn Doe")
	testutils.Equal(t, err, nil)

	if id2 == "" {
		t.Fatalf("We must have unempty bucket, got: %s", id2)
	}

	testutils.Equal(t, id1, id2)

	id3, err := service.Upsert(ctx, "users", "Jane Doe")
	testutils.Equal(t, err, nil)

	if id3 == "" {
		t.Fatalf("We must have unempty bucket, got: %s", id2)
	}

	for i := 0; i < 1000; i++ {
		_, err := service.Upsert(ctx, "users", "test"+strconv.Itoa(i)+"@gmail.com")
		testutils.Equal(t, err, nil)

		_, err = service.Upsert(ctx, "users", "example"+strconv.Itoa(i)+"@similar.com")
		testutils.Equal(t, err, nil)
	}

	pp.Print(repo)

	testutils.NotEqual(t, id1, id3)
}

func TestAerospike(t *testing.T) {
	t.Skip() // this test only to understand how aerospike works

	// That`s only to fill soma data
	ctx := context.Background()

	client, aerr := as.NewClient("127.0.0.1", 3000)
	testutils.Equal(t, aerr, nil)

	repo := aerospike.NewRepository(client, "test", "lsh", 5000)

	cfg, err := GetLSHConfigFromEnv()
	testutils.Equal(t, err, nil)

	service := NewSimilarityService(repo, cfg)

	st := time.Now()
	_, err = service.Upsert(ctx, "users", "test1@gmail.com")
	testutils.Equal(t, err, nil)

	pp.Println(time.Since(st))
}

func TestUpsert_EdgeCases(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name    string
		group   string
		input   string
		wantErr error
	}{
		{name: "empty input", group: "g", input: "", wantErr: ErrEmptyInputString},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewRepository()
			cfg := &Config{
				Bands: 20, Rows: 5, ShingleSize: 5,
				JaccardThreshold: 0.6, MaxBucketSize: 200,
				MaxTotalCandidates: 100, Seed: 42,
			}
			svc := NewSimilarityService(repo, cfg)

			_, err := svc.Upsert(ctx, tc.group, tc.input)
			testutils.Equal(t, err, tc.wantErr)
		})
	}
}

func TestUpsert_MaxBucketSizeSkip(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := &Config{
		Bands: 5, Rows: 2, ShingleSize: 3,
		JaccardThreshold: 0.6, MaxBucketSize: 2,
		MaxTotalCandidates: 100, Seed: 42,
	}
	svc := NewSimilarityService(repo, cfg)

	for i := 0; i < 10; i++ {
		_, err := svc.Upsert(ctx, "grp", "user"+strconv.Itoa(i)+"@example.com")
		testutils.Equal(t, err, nil)
	}

	id, err := svc.Upsert(ctx, "grp", "completely_different_string_xyz")
	testutils.Equal(t, err, nil)

	if id == "" {
		t.Fatal("expected non-empty id")
	}
}

// BenchmarkUpsert measures the performance of the Upsert operation.
// We test two scenarios:
// 1. New Record (Insertion cost)
// 2. Duplicate Record (Deduplication / Comparison cost)
func BenchmarkUpsert(b *testing.B) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := &Config{
		Bands:              20,
		Rows:               5,
		ShingleSize:        3,
		JaccardThreshold:   0.6,
		MaxBucketSize:      100,
		MaxTotalCandidates: 100,
		Seed:               42,
	}
	service := NewSimilarityService(repo, cfg)

	// Pre-seed some data to ensure buckets aren't empty
	for i := 0; i < 1000; i++ {
		_, _ = service.Upsert(ctx, "users", "seed"+strconv.Itoa(i)+"@gmail.com")
	}

	b.Run("NewRecord", func(b *testing.B) {
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Using a changing string to ensure it's always a "New" insert
			input := fmt.Sprintf("bench_new_%d@example.com", i)
			_, _ = service.Upsert(ctx, "users", input)
		}
	})

	b.Run("DuplicateMatch", func(b *testing.B) {
		// Ensure the target exists
		target := "duplicate@example.com"
		_, err := service.Upsert(ctx, "users", target)
		testutils.Equal(b, err, nil)

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Constant string to trigger the LSH match logic
			_, err = service.Upsert(ctx, "users", target)
			testutils.Equal(b, err, nil)
		}
	})
}

func TestUpsert_DeterministicID_Optimization(t *testing.T) {
	ctx := context.Background()
	repo := newSpyRepository()
	cfg, err := GetLSHConfigFromEnv()
	testutils.Equal(t, err, nil)

	service := NewSimilarityService(repo, cfg)

	input := "optimization_test_input"

	// 1. First Upsert: Should perform a full write.
	id1, err := service.Upsert(ctx, "test_group", input)
	testutils.Equal(t, err, nil)

	// Assert that the write methods were called exactly once.
	testutils.Equal(t, int64(1), atomic.LoadInt64(&repo.saveRecordCalls))
	testutils.Equal(t, int64(1), atomic.LoadInt64(&repo.batchAddToBucketCalls))

	// 2. Second Upsert: Should hit the optimization path and return early.
	id2, err := service.Upsert(ctx, "test_group", input)
	testutils.Equal(t, err, nil)

	// Assert that the ID is the same and the write methods were NOT called again.
	testutils.Equal(t, id1, id2)
	testutils.Equal(t, int64(1), atomic.LoadInt64(&repo.saveRecordCalls))
	testutils.Equal(t, int64(1), atomic.LoadInt64(&repo.batchAddToBucketCalls))
}

// BenchmarkFastSimilarityMath compares signature estimation vs raw jaccard
func BenchmarkSimilarityComparison(b *testing.B) {
	s1 := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	s2 := []uint64{1, 2, 3, 4, 0, 0, 0, 0, 0, 0} // 40% match

	b.Run("SignatureEstimation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = EstimateJaccard(s1, s2)
		}
	})
}

// spyRepository wraps a memory.Repository to count method calls for testing.
type spyRepository struct {
	*memory.Repository
	saveRecordCalls       int64
	batchAddToBucketCalls int64
}

func newSpyRepository() *spyRepository {
	return &spyRepository{
		Repository: memory.NewRepository(),
	}
}

func (s *spyRepository) SaveRecord(u model.Record) error {
	atomic.AddInt64(&s.saveRecordCalls, 1)
	return s.Repository.SaveRecord(u)
}

func (s *spyRepository) BatchAddToBuckets(bucketKeys []string, value string, length int) error {
	atomic.AddInt64(&s.batchAddToBucketCalls, 1)
	return s.Repository.BatchAddToBuckets(bucketKeys, value, length)
}
