package lsh

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	as "github.com/aerospike/aerospike-client-go/v8"
	"github.com/k0kubun/pp/v3"

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

func TestUpsert_DeterministicID_Optimization(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg, err := GetLSHConfigFromEnv()
	testutils.Equal(t, err, nil)

	service := NewSimilarityService(repo, cfg)

	input := "optimization_test_input"

	// First Upsert
	id1, err := service.Upsert(ctx, "test_group", input)
	testutils.Equal(t, err, nil)

	// Second Upsert (should hit optimization and return same ID)
	id2, err := service.Upsert(ctx, "test_group", input)
	testutils.Equal(t, err, nil)

	testutils.Equal(t, id1, id2)
}

func TestUpsert_Concurrency_SignatureIntegrity(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()

	cfg := &Config{
		Bands:              20,
		Rows:               5,
		ShingleSize:        3,
		JaccardThreshold:   0.9,
		MaxBucketSize:      100,
		MaxTotalCandidates: 100,
		Seed:               42,
	}

	service := NewSimilarityService(repo, cfg)

	count := 100
	ids := make([]string, count)
	inputs := make([]string, count)

	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Use a unique suffix to ensure we are creating new records
			input := fmt.Sprintf("concurrent_test_input_%d_unique_suffix", idx)
			id, err := service.Upsert(ctx, "concurrency_group", input)
			if err != nil {
				t.Errorf("Upsert failed: %v", err)
				return
			}

			mu.Lock()
			ids[idx] = id
			inputs[idx] = input
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify signatures
	records, err := repo.GetRecords(ids)
	if err != nil {
		t.Fatalf("Failed to get records: %v", err)
	}

	hasher := NewHasher(cfg.Bands, cfg.Rows, cfg.Seed)
	sigSize := cfg.Bands * cfg.Rows

	for i, id := range ids {
		rec, ok := records[id]
		if !ok {
			t.Errorf("Record not found for id: %s (index %d)", id, i)
			continue
		}

		if rec.Input != inputs[i] {
			t.Errorf("Input mismatch for id %s. Got %s, want %s", id, rec.Input, inputs[i])
		}

		// Re-compute signature to verify the stored signature is correct and wasn't corrupted
		// by race conditions in the pool usage.
		tokens := service.Shingle(inputs[i])
		expectedSig := make([]uint64, sigSize)
		hasher.ComputeSignature(tokens, expectedSig)

		if len(rec.Signature) != len(expectedSig) {
			t.Errorf("Signature length mismatch for input %s", inputs[i])
			continue
		}

		for j := range expectedSig {
			if rec.Signature[j] != expectedSig[j] {
				t.Errorf("Signature mismatch at index %d for input '%s'. Possible pool corruption.", j, inputs[i])
				break
			}
		}
	}
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
