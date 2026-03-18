//go:build integration

package vector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	as "github.com/aerospike/aerospike-client-go/v8"

	"github.com/FrogoAI/lsh/v2"
	asrepo "github.com/FrogoAI/lsh/v2/repositories/aerospike"
	"github.com/FrogoAI/lsh/v2/testdata"
)

func TestMain(m *testing.M) {
	if err := testdata.WaitForAerospike(90 * time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "aerospike: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func aerospikeRepo(t *testing.T) *asrepo.Repository {
	t.Helper()

	client, err := as.NewClient(testdata.Host, testdata.Port)
	if err != nil {
		t.Fatalf("failed to connect to Aerospike: %v", err)
	}

	t.Cleanup(func() { client.Close() })

	return asrepo.NewRepository(client, testdata.Namespace, uniqueSet())
}

func uniqueSet() string {
	var b [4]byte

	_, _ = rand.Read(b[:])

	return "v_" + hex.EncodeToString(b[:])
}

func integrationConfig(dims int) *Config {
	return &Config{
		Config: lsh.Config{
			Bands: 20, Rows: 5,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 42,
		},
		VectorDimensions: dims,
		CosineThreshold:  0.7,
	}
}

func TestVectorUpsert_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()
	cfg := integrationConfig(5)

	svc, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	vec := []float64{1.0, 2.0, 3.0, 4.0, 5.0}

	id, err := svc.Upsert(ctx, "grp", vec)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if id == "" {
		t.Fatal("expected non-empty id")
	}

	// Same vector should return same ID
	id2, err := svc.Upsert(ctx, "grp", vec)
	if err != nil {
		t.Fatalf("Upsert duplicate: %v", err)
	}

	if id != id2 {
		t.Errorf("exact duplicate: got %s vs %s", id, id2)
	}
}

func TestVectorUpsert_SimilarVectors_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()
	cfg := integrationConfig(5)
	cfg.CosineThreshold = 0.5

	svc, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	v1 := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	v2 := []float64{1.1, 2.1, 3.1, 4.1, 5.1}

	cos := ExactCosine(v1, v2)
	t.Logf("cos(v1, v2) = %.4f", cos)

	id1, err := svc.Upsert(ctx, "grp", v1)
	if err != nil {
		t.Fatalf("Upsert v1: %v", err)
	}

	id2, err := svc.Upsert(ctx, "grp", v2)
	if err != nil {
		t.Fatalf("Upsert v2: %v", err)
	}

	if id1 != id2 {
		t.Errorf("similar vectors should share ID (cos=%.3f): got %s vs %s", cos, id1, id2)
	}
}

func TestVectorUpsert_DifferentVectors_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()
	cfg := integrationConfig(5)
	cfg.CosineThreshold = 0.9

	svc, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	v1 := []float64{1.0, 0.0, 0.0, 0.0, 0.0}
	v2 := []float64{0.0, 0.0, 0.0, 0.0, 1.0}

	cos := ExactCosine(v1, v2)
	t.Logf("cos(v1, v2) = %.4f", cos)

	id1, _ := svc.Upsert(ctx, "grp", v1)
	id2, _ := svc.Upsert(ctx, "grp", v2)

	if id1 == id2 {
		t.Error("orthogonal vectors should get different IDs")
	}
}

func TestVectorUpsert_BehavioralScenarios_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()
	cfg := integrationConfig(5)
	cfg.CosineThreshold = 0.9

	svc, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Two bonus hunters
	hunter1 := []float64{2.0, 1.0, 0.95, 1.1, 1.0}
	hunter2 := []float64{3.0, 1.0, 0.98, 1.15, 1.0}

	cos := ExactCosine(hunter1, hunter2)
	t.Logf("cos(hunter1, hunter2) = %.4f", cos)

	id1, _ := svc.Upsert(ctx, "org1", hunter1)
	id2, _ := svc.Upsert(ctx, "org1", hunter2)

	if id1 != id2 {
		t.Errorf("bonus hunters should cluster: got %s vs %s", id1, id2)
	}

	// Normal user vs laundering pattern
	normal := []float64{20.0, 5.0, 0.3, 8.0, 0.0}
	launderer := []float64{5.0, 5.0, 0.98, 0.1, 0.0}

	id3, _ := svc.Upsert(ctx, "org1", normal)
	id4, _ := svc.Upsert(ctx, "org1", launderer)

	if id3 == id4 {
		t.Error("normal and launderer should get different IDs")
	}
}

func TestVectorUpsert_L2CachePersistence_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()
	cfg := integrationConfig(4)
	cfg.CosineThreshold = 0.5

	svc1, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	v1 := []float64{1.0, 2.0, 3.0, 4.0}
	v2 := []float64{1.1, 2.1, 3.1, 4.1}

	originalID, _ := svc1.Upsert(ctx, "grp", v1)
	_, _ = svc1.Upsert(ctx, "grp", v2)

	// Simulate pod restart
	svc2, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	id, err := svc2.Upsert(ctx, "grp", v2)
	if err != nil {
		t.Fatalf("Upsert after restart: %v", err)
	}

	if id != originalID {
		t.Errorf("L2 cache: expected %s, got %s", originalID, id)
	}
}

func TestVectorUpsert_PrecisionGuarantee_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()
	cfg := integrationConfig(10)
	cfg.CosineThreshold = 0.7

	svc, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Seed clusters
	clusters := 20

	for c := 0; c < clusters; c++ {
		vec := make([]float64, 10)
		for d := 0; d < 10; d++ {
			vec[d] = math.Sin(float64(c*7+d) * 0.37)
		}

		_, err := svc.Upsert(ctx, "grp", vec)
		if err != nil {
			t.Fatalf("cluster %d: %v", c, err)
		}
	}

	// Upsert noisy variants and verify precision
	violations := 0

	for c := 0; c < clusters; c++ {
		vec := make([]float64, 10)
		for d := 0; d < 10; d++ {
			vec[d] = math.Sin(float64(c*7+d)*0.37) + math.Sin(float64(999*13+d)*0.73)*0.05
		}

		id, err := svc.Upsert(ctx, "grp", vec)
		if err != nil {
			t.Fatalf("noisy %d: %v", c, err)
		}

		// Fetch the representative's record and verify cosine
		recs, _ := repo.GetRecords([]string{id})
		if len(recs) == 0 {
			continue
		}

		rec, ok := recordFromBins(recs[0])
		if !ok {
			continue
		}

		cos := ExactCosine(vec, rec.Vector)
		if cos < cfg.CosineThreshold {
			violations++
			t.Errorf("PRECISION VIOLATION: cos=%.3f < %.1f", cos, cfg.CosineThreshold)
		}
	}

	t.Logf("Precision violations: %d/%d", violations, clusters)
}

func TestVectorUpsert_Scale_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()
	cfg := integrationConfig(10)

	svc, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	for i := 0; i < 200; i++ {
		vec := make([]float64, 10)
		for d := 0; d < 10; d++ {
			vec[d] = math.Sin(float64(i*7+d) * 0.37)
		}

		_, err := svc.Upsert(ctx, "grp", vec)
		if err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}
}

func TestVectorUpsert_1000Vectors_GroupValidation_Aerospike(t *testing.T) {
	repo := aerospikeRepo(t)
	ctx := context.Background()
	dims := 10
	cfg := integrationConfig(dims)
	cfg.CosineThreshold = 0.8

	svc, err := NewService(repo, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Define 5 behavioral clusters with clear angular separation.
	// Cosine similarity ignores magnitude, so clusters must differ in DIRECTION.
	clusters := []struct {
		name string
		base []float64 // cluster centroid
	}{
		{name: "bonus_hunter", base: []float64{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.2}},
		{name: "money_launderer", base: []float64{0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.2, 0.0}},
		{name: "normal_player", base: []float64{0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 0.2, 0.0, 0.0}},
		{name: "high_roller", base: []float64{0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.2, 0.0, 0.0, 0.0}},
		{name: "affiliate_bot", base: []float64{0.0, 0.0, 0.0, 0.0, 1.0, 0.2, 0.0, 0.0, 0.0, 0.0}},
	}

	usersPerCluster := 200
	groupIDs := make([]map[string]int, len(clusters))

	for c, cl := range clusters {
		groupIDs[c] = make(map[string]int)

		for i := 0; i < usersPerCluster; i++ {
			vec := make([]float64, dims)

			for d := 0; d < dims; d++ {
				// Small random noise per user (deterministic from i and d)
				noise := math.Sin(float64(i*13+d)*0.73) * 0.03 * math.Abs(cl.base[d]+1)
				vec[d] = cl.base[d] + noise
			}

			id, err := svc.Upsert(ctx, "org1", vec)
			if err != nil {
				t.Fatalf("cluster=%s user=%d: %v", cl.name, i, err)
			}

			groupIDs[c][id]++
		}
	}

	// Validate: each cluster should produce few distinct IDs
	for c, cl := range clusters {
		distinct := len(groupIDs[c])

		t.Logf("Cluster %q: %d users -> %d distinct IDs (%.1f%% clustering)",
			cl.name, usersPerCluster, distinct,
			float64(usersPerCluster-distinct)/float64(usersPerCluster)*100)

		if distinct > usersPerCluster/2 {
			t.Errorf("Cluster %q: too many distinct IDs (%d/%d) — poor clustering",
				cl.name, distinct, usersPerCluster)
		}
	}

	// Validate: different clusters should NOT share IDs
	for i := 0; i < len(clusters); i++ {
		for j := i + 1; j < len(clusters); j++ {
			shared := 0

			for id := range groupIDs[i] {
				if _, ok := groupIDs[j][id]; ok {
					shared++
				}
			}

			if shared > 0 {
				t.Errorf("Clusters %q and %q share %d IDs — should be separate",
					clusters[i].name, clusters[j].name, shared)
			}
		}
	}
}
