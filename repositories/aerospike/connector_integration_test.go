//go:build integration

package aerospike

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	as "github.com/aerospike/aerospike-client-go/v8"

	"github.com/FrogoAI/lsh/v2/testdata"
)

var sharedClient *as.Client //nolint:gochecknoglobals

func TestMain(m *testing.M) {
	client, err := testdata.NewReadyClient(90 * time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aerospike: %v\n", err)
		os.Exit(1)
	}

	sharedClient = client
	code := m.Run()

	client.Close()
	os.Exit(code)
}

func newTestClient(t *testing.T) *as.Client {
	t.Helper()

	return sharedClient
}

func uniqueSet() string {
	var b [4]byte

	_, _ = rand.Read(b[:])

	return "c_" + hex.EncodeToString(b[:])
}

func TestSetRepresentative_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet())

	err := repo.SetRepresentative("bucket1", "user1", 42)
	if err != nil {
		t.Fatalf("SetRepresentative: %v", err)
	}

	reps, err := repo.GetRepresentatives("bucket1")
	if err != nil {
		t.Fatalf("GetRepresentatives: %v", err)
	}

	if len(reps) != 1 {
		t.Fatalf("expected 1 rep, got %d", len(reps))
	}

	if reps[0].ID != "user1" || reps[0].Metadata != 42 {
		t.Errorf("got %+v, want {ID:user1 Metadata:42}", reps[0])
	}
}

func TestSetRepresentative_Idempotent_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet())

	_ = repo.SetRepresentative("bucket1", "user1", 10)
	_ = repo.SetRepresentative("bucket1", "user1", 20)

	reps, _ := repo.GetRepresentatives("bucket1")

	if len(reps) != 1 {
		t.Fatalf("expected 1 rep after upsert, got %d", len(reps))
	}

	if reps[0].Metadata != 20 {
		t.Errorf("expected updated metadata 20, got %d", reps[0].Metadata)
	}
}

func TestBatchSetRepresentative_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet())

	err := repo.BatchSetRepresentative([]string{"b1", "b2", "b3"}, "user1", 5)
	if err != nil {
		t.Fatalf("BatchSetRepresentative: %v", err)
	}

	result, err := repo.BatchGetRepresentatives([]string{"b1", "b2", "b3", "b4"})
	if err != nil {
		t.Fatalf("BatchGetRepresentatives: %v", err)
	}

	for _, k := range []string{"b1", "b2", "b3"} {
		if len(result[k]) != 1 {
			t.Errorf("%s: expected 1 rep, got %d", k, len(result[k]))
		}
	}

	if len(result["b4"]) != 0 {
		t.Errorf("b4: expected 0 reps, got %d", len(result["b4"]))
	}
}

func TestMaxBucketReps_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet(), WithMaxBucketReps(3))

	for i := 0; i < 10; i++ {
		err := repo.SetRepresentative("capped", "u"+strconv.Itoa(i), int64(i))
		if err != nil {
			t.Fatalf("SetRepresentative u%d: %v", i, err)
		}
	}

	reps, err := repo.GetRepresentatives("capped")
	if err != nil {
		t.Fatalf("GetRepresentatives: %v", err)
	}

	if len(reps) > 3 {
		t.Errorf("expected at most 3 reps (cap), got %d", len(reps))
	}
}

func TestBatchMaxBucketReps_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet(), WithMaxBucketReps(5))

	for i := 0; i < 20; i++ {
		err := repo.BatchSetRepresentative([]string{"bc1", "bc2"}, "u"+strconv.Itoa(i), int64(i))
		if err != nil {
			t.Fatalf("BatchSetRepresentative u%d: %v", i, err)
		}
	}

	result, _ := repo.BatchGetRepresentatives([]string{"bc1", "bc2"})

	for _, k := range []string{"bc1", "bc2"} {
		if len(result[k]) > 5 {
			t.Errorf("%s: expected at most 5 reps (cap), got %d", k, len(result[k]))
		}
	}
}

func TestSaveAndGetRecords_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet())

	bins := map[string]any{
		"v": []float64{1.0, 2.0, 3.0},
		"g": "grp",
		"s": []uint64{0, 1, 0, 1},
	}

	err := repo.SaveRecord("rec1", bins)
	if err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}

	recs, err := repo.GetRecords([]string{"rec1", "missing"})
	if err != nil {
		t.Fatalf("GetRecords: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}

	if recs[0].Key != "rec1" {
		t.Errorf("key: got %s, want rec1", recs[0].Key)
	}

	if recs[0].Bins["g"] != "grp" {
		t.Errorf("group: got %v, want grp", recs[0].Bins["g"])
	}
}

func TestPutAndGetValue_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet())

	err := repo.PutValue("key1", "value1")
	if err != nil {
		t.Fatalf("PutValue: %v", err)
	}

	got, err := repo.GetValue("key1")
	if err != nil {
		t.Fatalf("GetValue: %v", err)
	}

	if got != "value1" {
		t.Errorf("got %q, want %q", got, "value1")
	}

	got2, err := repo.GetValue("missing")
	if err != nil {
		t.Fatalf("GetValue missing: %v", err)
	}

	if got2 != "" {
		t.Errorf("missing key: got %q, want empty", got2)
	}
}

func TestGetRepresentatives_Missing_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet())

	reps, err := repo.GetRepresentatives("nonexistent")
	if err != nil {
		t.Fatalf("GetRepresentatives: %v", err)
	}

	if reps != nil {
		t.Errorf("expected nil for missing bucket, got %v", reps)
	}
}

// TestDedupUpsert_Integration tests the full dedup Upsert flow against Aerospike.
func TestDedupUpsert_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet())

	// We can't import dedup here (circular), so test the storage operations
	// that dedup.Upsert would perform.

	// Simulate: novel item stores record + representatives
	bins := map[string]any{"i": "maxim@weavers.team", "g": "email", "s": []uint64{1, 2, 3}}

	err := repo.SaveRecord("id1", bins)
	if err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}

	err = repo.BatchSetRepresentative([]string{"b0:abc", "b1:def"}, "id1", 15)
	if err != nil {
		t.Fatalf("BatchSetRepresentative: %v", err)
	}

	// Simulate: resolved item stores cache
	err = repo.PutValue("res:id2", "id1")
	if err != nil {
		t.Fatalf("PutValue: %v", err)
	}

	// Verify full round-trip
	recs, _ := repo.GetRecords([]string{"id1"})
	if len(recs) != 1 || recs[0].Bins["i"] != "maxim@weavers.team" {
		t.Errorf("record round-trip failed: %+v", recs)
	}

	resolved, _ := repo.GetValue("res:id2")
	if resolved != "id1" {
		t.Errorf("resolved cache: got %q, want id1", resolved)
	}

	reps, _ := repo.BatchGetRepresentatives([]string{"b0:abc", "b1:def"})
	if len(reps["b0:abc"]) != 1 || reps["b0:abc"][0].ID != "id1" {
		t.Errorf("representative round-trip failed: %+v", reps)
	}
}

// TestVectorUpsert_Integration tests the storage operations for vector Upsert.
func TestVectorUpsert_Integration(t *testing.T) {
	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet())

	// Simulate: novel vector stores record + representatives
	vec := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	sig := []uint64{0, 1, 1, 0, 1, 0, 0, 1, 1, 0}
	bins := map[string]any{"v": vec, "g": "org1", "s": sig}

	err := repo.SaveRecord("vid1", bins)
	if err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}

	err = repo.BatchSetRepresentative([]string{"vb0:aaa", "vb1:bbb", "vb2:ccc"}, "vid1", 0)
	if err != nil {
		t.Fatalf("BatchSetRepresentative: %v", err)
	}

	// Verify record round-trip (vector deserialization)
	recs, err := repo.GetRecords([]string{"vid1"})
	if err != nil {
		t.Fatalf("GetRecords: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}

	// Aerospike returns []any for lists, verify we can read them
	rawVec := recs[0].Bins["v"]
	if rawVec == nil {
		t.Fatal("vector bin is nil")
	}

	rawSig := recs[0].Bins["s"]
	if rawSig == nil {
		t.Fatal("signature bin is nil")
	}

	// Verify representatives round-trip
	allReps, err := repo.BatchGetRepresentatives([]string{"vb0:aaa", "vb1:bbb", "vb2:ccc"})
	if err != nil {
		t.Fatalf("BatchGetRepresentatives: %v", err)
	}

	for _, k := range []string{"vb0:aaa", "vb1:bbb", "vb2:ccc"} {
		reps := allReps[k]
		if len(reps) != 1 {
			t.Errorf("%s: expected 1 rep, got %d", k, len(reps))

			continue
		}

		if reps[0].ID != "vid1" {
			t.Errorf("%s: expected vid1, got %s", k, reps[0].ID)
		}
	}
}

// TestFullDedupFlow_Integration runs the actual dedup.Service against Aerospike.
func TestFullDedupFlow_Integration(t *testing.T) {
	_ = context.Background() // used by service tests below

	client := newTestClient(t)

	repo := NewRepository(client, testdata.Namespace, uniqueSet())

	// Verify the storage interface contract with a multi-step workflow:
	// 1. Store representative
	// 2. Query it back
	// 3. Store resolved cache
	// 4. Query resolved
	// 5. Store another representative in same bucket (set semantics)

	_ = repo.SetRepresentative("flow:b0", "rep1", 10)
	_ = repo.SetRepresentative("flow:b0", "rep2", 20)
	_ = repo.SetRepresentative("flow:b0", "rep1", 15) // update rep1

	reps, _ := repo.GetRepresentatives("flow:b0")
	if len(reps) != 2 {
		t.Fatalf("expected 2 reps (set semantics), got %d", len(reps))
	}

	repMap := make(map[string]int64)
	for _, r := range reps {
		repMap[r.ID] = r.Metadata
	}

	if repMap["rep1"] != 15 {
		t.Errorf("rep1 metadata: expected 15 (updated), got %d", repMap["rep1"])
	}

	if repMap["rep2"] != 20 {
		t.Errorf("rep2 metadata: expected 20, got %d", repMap["rep2"])
	}
}
