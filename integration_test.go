//go:build integration

package lsh

import (
	"context"
	"testing"

	as "github.com/aerospike/aerospike-client-go/v8"

	"github.com/FrogoAI/lsh/repositories/aerospike"
	"github.com/FrogoAI/testutils"
)

const (
	aerospikeHost      = "127.0.0.1"
	aerospikePort      = 3000
	aerospikeNamespace = "test"
	aerospikeSet       = "lsh_integration"
)

func newAerospikeRepo(t *testing.T) *aerospike.Repository {
	t.Helper()

	client, err := as.NewClient(aerospikeHost, aerospikePort)
	if err != nil {
		t.Fatalf("failed to connect to aerospike: %v", err)
	}

	t.Cleanup(func() { client.Close() })

	return aerospike.NewRepository(client, aerospikeNamespace, aerospikeSet, 5000)
}

func TestAerospike_Upsert(t *testing.T) {
	repo := newAerospikeRepo(t)
	cfg := &Config{
		Bands: 40, Rows: 5, ShingleSize: 3,
		JaccardThreshold: 0.6, MaxBucketSize: 200,
		MaxTotalCandidates: 100, Seed: 13374269,
	}

	ctx := context.Background()
	svc := NewSimilarityService(repo, cfg)

	cases := []struct {
		name      string
		input     string
		wantSameAs string
	}{
		{name: "original", input: "maxim@weavers.team"},
		{name: "typo in domain", input: "maxim@weavets.team", wantSameAs: "original"},
		{name: "extra char", input: "maxim@weavers.teams", wantSameAs: "original"},
		{name: "typo in name", input: "maxem@weavers.teams", wantSameAs: "original"},
		{name: "completely different", input: "john@example.com"},
	}

	ids := make(map[string]string)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, err := svc.Upsert(ctx, "email", tc.input)
			testutils.Equal(t, err, nil)

			if id == "" {
				t.Fatal("expected non-empty id")
			}

			ids[tc.name] = id

			if tc.wantSameAs != "" {
				testutils.Equal(t, ids[tc.wantSameAs], id)
			}
		})
	}

	// "completely different" must not match "original"
	testutils.NotEqual(t, ids["original"], ids["completely different"])
}

func TestAerospike_ResolvedCache(t *testing.T) {
	repo := newAerospikeRepo(t)
	cfg := &Config{
		Bands: 40, Rows: 5, ShingleSize: 3,
		JaccardThreshold: 0.6, MaxBucketSize: 200,
		MaxTotalCandidates: 100, Seed: 13374269,
	}

	ctx := context.Background()

	// Pod 1: resolve similar input via LSH, populates L2
	svc1 := NewSimilarityService(repo, cfg)

	originalID, err := svc1.Upsert(ctx, "email", "cache_test@weavers.team")
	testutils.Equal(t, err, nil)

	similarID, err := svc1.Upsert(ctx, "email", "cache_test@weavets.team")
	testutils.Equal(t, err, nil)
	testutils.Equal(t, originalID, similarID)

	// Pod 2 (simulated restart): new service instance, same repo
	svc2 := NewSimilarityService(repo, cfg)

	// Should hit L2 persistent cache, no full LSH needed
	cachedID, err := svc2.Upsert(ctx, "email", "cache_test@weavets.team")
	testutils.Equal(t, err, nil)
	testutils.Equal(t, originalID, cachedID)
}

func TestAerospike_SaveAndGetResolvedID(t *testing.T) {
	repo := newAerospikeRepo(t)

	cases := []struct {
		name       string
		bid        string
		resolvedID string
	}{
		{name: "basic mapping", bid: "input_hash_1", resolvedID: "resolved_hash_1"},
		{name: "another mapping", bid: "input_hash_2", resolvedID: "resolved_hash_2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := repo.SaveResolvedID(tc.bid, tc.resolvedID)
			testutils.Equal(t, err, nil)

			got, err := repo.GetResolvedID(tc.bid)
			testutils.Equal(t, err, nil)
			testutils.Equal(t, tc.resolvedID, got)
		})
	}

	// Non-existent key returns empty string
	got, err := repo.GetResolvedID("nonexistent")
	testutils.Equal(t, err, nil)
	testutils.Equal(t, "", got)
}
