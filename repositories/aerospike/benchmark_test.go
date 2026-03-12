//go:build integration

package aerospike

import (
	"fmt"
	"testing"

	as "github.com/aerospike/aerospike-client-go/v8"

	"github.com/FrogoAI/lsh/model"
)

func newBenchRepo(b *testing.B) *Repository {
	b.Helper()

	client, err := as.NewClient("127.0.0.1", 3000)
	if err != nil {
		b.Fatalf("failed to connect to aerospike: %v", err)
	}

	b.Cleanup(func() { client.Close() })

	return NewRepository(client, "test", "lsh_bench", 5000)
}

func BenchmarkAerospike_AddToBucket(b *testing.B) {
	repo := newBenchRepo(b)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = repo.AddToBucket("bench_bucket", fmt.Sprintf("user_%d", i), i)
	}
}

func BenchmarkAerospike_GetBucketMembers(b *testing.B) {
	repo := newBenchRepo(b)

	for i := 0; i < 100; i++ {
		_ = repo.AddToBucket("bench_get_bucket", fmt.Sprintf("user_%d", i), i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = repo.GetBucketMembers("bench_get_bucket")
	}
}

func BenchmarkAerospike_SaveRecord(b *testing.B) {
	repo := newBenchRepo(b)

	sig := make([]uint64, 200)
	for i := range sig {
		sig[i] = uint64(i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = repo.SaveRecord(model.Record{
			ID:        fmt.Sprintf("bench_rec_%d", i),
			Input:     fmt.Sprintf("bench_input_%d@example.com", i),
			GroupID:   "bench",
			Signature: sig,
		})
	}
}

func BenchmarkAerospike_GetRecords(b *testing.B) {
	repo := newBenchRepo(b)

	sig := make([]uint64, 200)
	ids := make([]string, 10)

	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("bench_get_rec_%d", i)
		ids[i] = id

		_ = repo.SaveRecord(model.Record{
			ID: id, Input: "test", GroupID: "bench", Signature: sig,
		})
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = repo.GetRecords(ids)
	}
}

func BenchmarkAerospike_BatchAddToBuckets(b *testing.B) {
	repo := newBenchRepo(b)

	keys := make([]string, 40)
	for i := range keys {
		keys[i] = fmt.Sprintf("bench_batch_b%d", i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = repo.BatchAddToBuckets(keys, fmt.Sprintf("user_%d", i), 10)
	}
}

func BenchmarkAerospike_BatchGetBuckets(b *testing.B) {
	repo := newBenchRepo(b)

	keys := make([]string, 40)
	for i := range keys {
		keys[i] = fmt.Sprintf("bench_bget_b%d", i)
	}

	_ = repo.BatchAddToBuckets(keys, "seed_user", 10)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = repo.BatchGetBuckets(keys)
	}
}

func BenchmarkAerospike_SaveResolvedID(b *testing.B) {
	repo := newBenchRepo(b)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = repo.SaveResolvedID(fmt.Sprintf("bid_%d", i), "resolved_abc")
	}
}

func BenchmarkAerospike_GetResolvedID(b *testing.B) {
	repo := newBenchRepo(b)

	_ = repo.SaveResolvedID("bench_res_bid", "bench_resolved")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = repo.GetResolvedID("bench_res_bid")
	}
}
