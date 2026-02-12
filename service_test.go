package lsh

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	as "github.com/aerospike/aerospike-client-go/v8"
	"github.com/k0kubun/pp/v3"

	"github.com/FrogoAI/lsh/repositories/aerospike"
	"github.com/FrogoAI/lsh/repositories/memory"
	"github.com/FrogoAI/testutils"
)

func TestUpserEmail(t *testing.T) {
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
		_, _ = service.Upsert(ctx, "users", target)

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Constant string to trigger the LSH match logic
			_, _ = service.Upsert(ctx, "users", target)
		}
	})
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
