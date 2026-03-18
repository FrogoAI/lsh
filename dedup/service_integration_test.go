//go:build integration

package dedup

import (
	"context"
	"testing"

	as "github.com/aerospike/aerospike-client-go/v8"

	"github.com/FrogoAI/lsh/v2"
	asrepo "github.com/FrogoAI/lsh/v2/repositories/aerospike"
)

func aerospikeRepo(t *testing.T) *asrepo.Repository {
	t.Helper()

	client, err := as.NewClient("127.0.0.1", 3000)
	if err != nil {
		t.Fatalf("failed to connect to Aerospike: %v", err)
	}

	t.Cleanup(func() { client.Close() })

	_ = client.Truncate(nil, "test", "dedup_int", nil)

	return asrepo.NewRepository(client, "test", "dedup_int")
}

func integrationConfig() *Config {
	return &Config{
		Config: lsh.Config{
			Bands: 40, Rows: 5,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 13374269,
		},
		ShingleSize:      3,
		JaccardThreshold: 0.6,
	}
}

func TestDedupUpsert_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()

	svc, err := NewService(repo, integrationConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id1, err := svc.Upsert(ctx, "email", "maxim@weavers.team")
	if err != nil {
		t.Fatalf("Upsert 1: %v", err)
	}

	if id1 == "" {
		t.Fatal("expected non-empty id")
	}

	// Similar string should match
	id2, err := svc.Upsert(ctx, "email", "maxim@weavets.team")
	if err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}

	if id1 != id2 {
		t.Errorf("similar strings should share ID: got %s vs %s", id1, id2)
	}

	// Different string should NOT match
	id3, err := svc.Upsert(ctx, "email", "completely_different_input@example.org")
	if err != nil {
		t.Fatalf("Upsert 3: %v", err)
	}

	if id1 == id3 {
		t.Error("dissimilar strings should get different IDs")
	}

	id4, err := svc.Upsert(ctx, "email", "completely_different_inpat@example.org")
	if err != nil {
		t.Fatalf("Upsert 3: %v", err)
	}

	if id4 != id3 {
		t.Error("similar new strings should get same IDs")
	}
}

func TestDedupUpsert_ExactDuplicate_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()

	svc, err := NewService(repo, integrationConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id1, _ := svc.Upsert(ctx, "grp", "exact_duplicate_test")
	id2, _ := svc.Upsert(ctx, "grp", "exact_duplicate_test")

	if id1 != id2 {
		t.Errorf("exact duplicate: got %s vs %s", id1, id2)
	}
}

func TestDedupUpsert_L2CachePersistence_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()
	cfg := integrationConfig()

	// Service 1: insert and resolve
	svc1, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	originalID, err := svc1.Upsert(ctx, "email", "maxim@weavers.team")
	if err != nil {
		t.Fatalf("Upsert original: %v", err)
	}

	resolvedID, err := svc1.Upsert(ctx, "email", "maxim@weavets.team")
	if err != nil {
		t.Fatalf("Upsert similar: %v", err)
	}

	if originalID != resolvedID {
		t.Fatalf("expected same ID, got %s vs %s", originalID, resolvedID)
	}

	// Service 2: simulate pod restart (new service, same repo)
	svc2, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id, err := svc2.Upsert(ctx, "email", "maxim@weavets.team")
	if err != nil {
		t.Fatalf("Upsert after restart: %v", err)
	}

	if id != originalID {
		t.Errorf("L2 cache failed: expected %s, got %s", originalID, id)
	}
}

func TestDedupUpsert_MultipleGroups_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()

	svc, err := NewService(repo, integrationConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id1, _ := svc.Upsert(ctx, "org1", "John Doe")
	id2, _ := svc.Upsert(ctx, "org2", "John Doe")

	// Same input in different groups should be independent
	// (they might get the same deterministic ID, but LSH runs independently)
	if id1 == "" || id2 == "" {
		t.Error("expected non-empty IDs")
	}
}

func TestDedupUpsert_Scale_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()

	svc, err := NewService(repo, integrationConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Insert 100 unique strings
	for i := 0; i < 100; i++ {
		input := "user" + string(rune('A'+i%26)) + string(rune('0'+i/26)) + "@test.com"

		_, err := svc.Upsert(ctx, "scale", input)
		if err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}
}

func TestDedupUpsert_1000Strings_GroupValidation_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()

	svc, err := NewService(repo, integrationConfig())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Define 5 groups of similar strings (200 per group)
	groups := []struct {
		prefix string
		count  int
	}{
		{prefix: "john.doe", count: 200},
		{prefix: "alice.smith", count: 200},
		{prefix: "bob.jones", count: 200},
		{prefix: "maria.garcia", count: 200},
		{prefix: "test.user.account.verification", count: 200},
	}

	// Track which IDs each group produces
	groupIDs := make([]map[string]int, len(groups))

	for g, grp := range groups {
		groupIDs[g] = make(map[string]int)

		for i := 0; i < grp.count; i++ {
			// Generate similar strings: small typos/variations
			input := grp.prefix
			switch i % 5 {
			case 0:
				input += "@gmail.com"
			case 1:
				input += "@gmaill.com" // typo
			case 2:
				input += "@gnail.com" // typo
			case 3:
				input += "@gmail.con" // typo
			case 4:
				input += "@gmail.co" // truncation
			}

			id, err := svc.Upsert(ctx, "groups", input)
			if err != nil {
				t.Fatalf("group=%d i=%d: %v", g, i, err)
			}

			groupIDs[g][id]++
		}
	}

	// Validate: each group should produce few distinct IDs (high clustering)
	for g, grp := range groups {
		distinct := len(groupIDs[g])
		total := grp.count

		t.Logf("Group %q: %d inputs -> %d distinct IDs (%.1f%% clustering)",
			grp.prefix, total, distinct, float64(total-distinct)/float64(total)*100)

		// Within a group, similar strings should mostly cluster
		if distinct > total/2 {
			t.Errorf("Group %q: too many distinct IDs (%d/%d) — poor clustering",
				grp.prefix, distinct, total)
		}
	}

	// Validate: different groups should NOT share IDs
	for i := 0; i < len(groups); i++ {
		for j := i + 1; j < len(groups); j++ {
			shared := 0

			for id := range groupIDs[i] {
				if _, ok := groupIDs[j][id]; ok {
					shared++
				}
			}

			if shared > 0 {
				t.Errorf("Groups %q and %q share %d IDs — should be separate clusters",
					groups[i].prefix, groups[j].prefix, shared)
			}
		}
	}
}
