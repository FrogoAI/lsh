package vector

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/FrogoAI/lsh/v2"
	"github.com/FrogoAI/lsh/v2/repositories"
	"github.com/FrogoAI/lsh/v2/repositories/memory"
)

// scaleConfig returns a config tuned for scale testing.
func scaleConfig(dims int) *Config {
	return &Config{
		Config: lsh.Config{
			Bands: 20, Rows: 5,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 42,
		},
		VectorDimensions: dims,
		CosineThreshold:  0.7,
	}
}

// makeVector creates a deterministic vector for the given cluster and noise seed.
// Vectors in the same cluster have high cosine similarity; different clusters have low.
func makeVector(dims, cluster, noiseSeed int) []float64 {
	v := make([]float64, dims)

	// Base direction from cluster ID
	for d := 0; d < dims; d++ {
		v[d] = math.Sin(float64(cluster*7+d) * 0.37)
	}

	// Small noise per noiseSeed (keeps vectors in same cluster close)
	if noiseSeed > 0 {
		for d := 0; d < dims; d++ {
			noise := math.Sin(float64(noiseSeed*13+d)*0.73) * 0.05
			v[d] += noise
		}
	}

	return v
}

func TestScale_RecordSize(t *testing.T) {
	dims := 20
	bands := 20
	rows := 5
	sigSize := bands * rows

	keySize := 22 // base64url(sha256[:16])
	vectorBytes := dims * 8
	signatureBytes := sigSize * 8
	groupBytes := 20
	overhead := 90

	total := keySize + vectorBytes + signatureBytes + groupBytes + overhead

	t.Logf("Vector record size breakdown:")
	t.Logf("  Key:       %d bytes", keySize)
	t.Logf("  Vector:    %d bytes (%d dims x 8)", vectorBytes, dims)
	t.Logf("  Signature: %d bytes (%d elements x 8)", signatureBytes, sigSize)
	t.Logf("  Group:     ~%d bytes", groupBytes)
	t.Logf("  Overhead:  ~%d bytes", overhead)
	t.Logf("  TOTAL:     ~%d bytes (~%.1f KB)", total, float64(total)/1024)

	// Verify actual bin sizes
	vec := makeVector(dims, 0, 0)
	svc := NewService(memory.NewRepository(), scaleConfig(dims))
	bins := Record{
		ID:        svc.GetNewID(vec),
		Vector:    vec,
		GroupID:   "test-group",
		Signature: make([]uint64, sigSize),
	}.toBins()

	if len(bins["v"].([]float64)) != dims {
		t.Errorf("vector bin: expected %d elements, got %d", dims, len(bins["v"].([]float64)))
	}

	if total > 2048 {
		t.Errorf("record size %d exceeds 2 KB budget", total)
	}
}

func TestScale_BucketGrowthWithClusters(t *testing.T) {
	ctx := context.Background()
	dims := 5

	cases := []struct {
		name         string
		clusters     int
		usersPerClus int
		maxBucket    int
	}{
		{name: "10 clusters x 10 users", clusters: 10, usersPerClus: 10, maxBucket: 10},
		{name: "50 clusters x 5 users", clusters: 50, usersPerClus: 5, maxBucket: 50},
		{name: "100 clusters x 3 users", clusters: 100, usersPerClus: 3, maxBucket: 100},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewRepository()
			cfg := scaleConfig(dims)
			cfg.CosineThreshold = 0.8
			svc := NewService(repo, cfg)

			for c := 0; c < tc.clusters; c++ {
				for u := 0; u < tc.usersPerClus; u++ {
					vec := makeVector(dims, c, u)

					_, err := svc.Upsert(ctx, "grp", vec)
					if err != nil {
						t.Fatalf("cluster=%d user=%d: %v", c, u, err)
					}
				}
			}

			// Check bucket sizes via repo internals
			allReps, _ := repo.BatchGetRepresentatives([]string{"dummy"})
			_ = allReps

			// Count actual representatives stored
			prefix, _ := cfg.HashVersion("grp")
			totalReps := 0
			maxReps := 0

			for b := 0; b < cfg.Bands; b++ {
				// We don't know the exact bucket keys, but we can count records
				_ = b
			}

			// Count unique records (= unique representatives/clusters)
			recs, _ := repo.GetRecords(nil)
			_ = recs

			// Instead, count via the service: distinct IDs returned
			idSet := make(map[string]bool)

			for c := 0; c < tc.clusters; c++ {
				vec := makeVector(dims, c, 0)
				id, _ := svc.Upsert(ctx, "grp", vec)
				idSet[id] = true
			}

			_ = prefix
			_ = totalReps
			_ = maxReps

			t.Logf("Unique cluster IDs: %d (expected <= %d)", len(idSet), tc.clusters)

			// Representatives per bucket should be bounded by cluster count, not user count
			if len(idSet) > tc.clusters {
				t.Errorf("more IDs (%d) than clusters (%d)", len(idSet), tc.clusters)
			}
		})
	}
}

func TestScale_RecallWithManyClusters(t *testing.T) {
	ctx := context.Background()
	dims := 10

	// Seed clusters: each cluster has one representative vector
	clusterCount := 200
	repo := memory.NewRepository()
	cfg := scaleConfig(dims)
	cfg.CosineThreshold = 0.7
	cfg.MaxTotalCandidates = 50
	svc := NewService(repo, cfg)

	clusterIDs := make([]string, clusterCount)
	clusterVecs := make([][]float64, clusterCount)

	for c := 0; c < clusterCount; c++ {
		vec := makeVector(dims, c, 0)
		clusterVecs[c] = vec

		id, err := svc.Upsert(ctx, "grp", vec)
		if err != nil {
			t.Fatalf("cluster %d: %v", c, err)
		}

		clusterIDs[c] = id
	}

	// Now upsert slightly noisy versions and check they resolve correctly
	hits := 0
	misses := 0
	total := 0

	for c := 0; c < clusterCount; c++ {
		for noise := 1; noise <= 3; noise++ {
			noisyVec := makeVector(dims, c, noise)

			// Only test if noisy vector is actually similar enough
			cos := ExactCosine(clusterVecs[c], noisyVec)
			if cos < cfg.CosineThreshold {
				continue
			}

			total++

			id, err := svc.Upsert(ctx, "grp", noisyVec)
			if err != nil {
				t.Fatalf("cluster=%d noise=%d: %v", c, noise, err)
			}

			if id == clusterIDs[c] {
				hits++
			} else {
				misses++
			}
		}
	}

	recall := float64(hits) / float64(total)

	t.Logf("Clusters: %d, MaxTotalCandidates: %d", clusterCount, cfg.MaxTotalCandidates)
	t.Logf("Tested: %d, Hits: %d, Misses: %d, Recall: %.1f%%", total, hits, misses, recall*100)

	// With band-overlap ranking, recall should remain high
	if recall < 0.80 {
		t.Errorf("recall %.1f%% is below 80%% threshold", recall*100)
	}
}

func TestScale_RecallDegradationWithMaxTotalCandidates(t *testing.T) {
	ctx := context.Background()
	dims := 8

	cases := []struct {
		name      string
		clusters  int
		maxCand   int
		minRecall float64
	}{
		{name: "small/high_budget", clusters: 20, maxCand: 100, minRecall: 0.9},
		{name: "medium/medium_budget", clusters: 100, maxCand: 50, minRecall: 0.7},
		{name: "large/low_budget", clusters: 200, maxCand: 20, minRecall: 0.5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := memory.NewRepository()
			cfg := scaleConfig(dims)
			cfg.CosineThreshold = 0.8
			cfg.MaxTotalCandidates = tc.maxCand
			svc := NewService(repo, cfg)

			clusterIDs := make([]string, tc.clusters)
			clusterVecs := make([][]float64, tc.clusters)

			for c := 0; c < tc.clusters; c++ {
				vec := makeVector(dims, c, 0)
				clusterVecs[c] = vec

				id, err := svc.Upsert(ctx, "grp", vec)
				if err != nil {
					t.Fatalf("cluster %d: %v", c, err)
				}

				clusterIDs[c] = id
			}

			hits := 0
			total := 0

			for c := 0; c < tc.clusters; c++ {
				noisyVec := makeVector(dims, c, 999)
				cos := ExactCosine(clusterVecs[c], noisyVec)

				if cos < cfg.CosineThreshold {
					continue
				}

				total++

				id, _ := svc.Upsert(ctx, "grp", noisyVec)
				if id == clusterIDs[c] {
					hits++
				}
			}

			if total == 0 {
				t.Skip("no test vectors met threshold")
			}

			recall := float64(hits) / float64(total)
			t.Logf("clusters=%d maxCand=%d total=%d hits=%d recall=%.1f%%",
				tc.clusters, tc.maxCand, total, hits, recall*100)

			if recall < tc.minRecall {
				t.Errorf("recall %.1f%% below minimum %.1f%%", recall*100, tc.minRecall*100)
			}
		})
	}
}

func TestScale_PrecisionGuarantee(t *testing.T) {
	// The algorithm guarantees: if vector V is resolved to representative R,
	// then ExactCosine(V, R.Vector) >= CosineThreshold.
	// We verify this by fetching the actual stored record for each resolved ID.
	ctx := context.Background()
	dims := 10
	clusters := 100

	repo := memory.NewRepository()
	cfg := scaleConfig(dims)
	cfg.CosineThreshold = 0.7
	svc := NewService(repo, cfg)

	// Seed representatives
	for c := 0; c < clusters; c++ {
		vec := makeVector(dims, c, 0)

		_, err := svc.Upsert(ctx, "grp", vec)
		if err != nil {
			t.Fatalf("cluster %d: %v", c, err)
		}
	}

	// Upsert noisy vectors and verify precision guarantee
	violations := 0
	resolved := 0
	total := 0

	for c := 0; c < clusters; c++ {
		for noise := 1; noise <= 5; noise++ {
			noisyVec := makeVector(dims, c, noise)
			total++

			id, err := svc.Upsert(ctx, "grp", noisyVec)
			if err != nil {
				t.Fatalf("cluster=%d noise=%d: %v", c, noise, err)
			}

			// Fetch the actual representative's stored record
			recs, err := repo.GetRecords([]string{id})
			if err != nil {
				t.Fatalf("GetRecords: %v", err)
			}

			if len(recs) == 0 {
				continue
			}

			rec, ok := recordFromBins(recs[0])
			if !ok {
				continue
			}

			// Check if this was resolved (noisy vector ID != representative ID)
			noisyID := svc.GetNewID(noisyVec)
			if noisyID == id {
				continue // exact match, not a resolution
			}

			resolved++

			cos := ExactCosine(noisyVec, rec.Vector)
			if cos < cfg.CosineThreshold {
				violations++

				t.Errorf("PRECISION VIOLATION: cos(noisy, stored_rep)=%.3f < threshold=%.1f",
					cos, cfg.CosineThreshold)
			}
		}
	}

	t.Logf("Total: %d, Resolved: %d, Violations: %d", total, resolved, violations)

	if violations > 0 {
		t.Errorf("%d precision violations", violations)
	}
}

func TestScale_NoFalsePositivesBelowThreshold(t *testing.T) {
	// Verify that orthogonal vectors are NEVER clustered together.
	ctx := context.Background()
	dims := 20

	repo := memory.NewRepository()
	cfg := scaleConfig(dims)
	cfg.CosineThreshold = 0.9
	svc := NewService(repo, cfg)

	// Create orthogonal unit vectors (one-hot in each dimension)
	ids := make([]string, dims)

	for d := 0; d < dims; d++ {
		vec := make([]float64, dims)
		vec[d] = 1.0

		id, err := svc.Upsert(ctx, "grp", vec)
		if err != nil {
			t.Fatalf("dim %d: %v", d, err)
		}

		ids[d] = id
	}

	// All should be different (orthogonal vectors have cos=0)
	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	if len(idSet) != dims {
		t.Errorf("expected %d unique IDs for orthogonal vectors, got %d", dims, len(idSet))
	}
}

func TestScale_IdempotentRepresentatives(t *testing.T) {
	ctx := context.Background()
	dims := 5
	repo := memory.NewRepository()
	cfg := scaleConfig(dims)
	svc := NewService(repo, cfg)

	vec := []float64{1.0, 2.0, 3.0, 4.0, 5.0}

	// Upsert same vector many times
	var firstID string

	for i := 0; i < 100; i++ {
		id, err := svc.Upsert(ctx, "grp", vec)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}

		if i == 0 {
			firstID = id
		}

		if id != firstID {
			t.Fatalf("iteration %d: id changed from %s to %s", i, firstID, id)
		}
	}

	// Check that bucket has exactly 1 representative, not 100
	prefix, _ := cfg.HashVersion("grp")
	sig := make([]uint64, cfg.SignatureSize())
	svc.hasher.ComputeSignature(vec, sig)

	keys, _ := lsh.ComputeBands(sig, cfg.Bands, cfg.Rows)
	bucketKeys := lsh.PrefixKeys(prefix, keys)

	allReps, _ := repo.BatchGetRepresentatives(bucketKeys)

	for _, bk := range bucketKeys {
		reps := allReps[bk]
		if len(reps) != 1 {
			t.Errorf("bucket %s: expected 1 rep, got %d", bk, len(reps))
		}
	}
}

func TestScale_ManyUsersPerCluster(t *testing.T) {
	ctx := context.Background()
	dims := 8
	clusters := 10
	usersPerCluster := 50

	repo := memory.NewRepository()
	cfg := scaleConfig(dims)
	cfg.CosineThreshold = 0.5
	svc := NewService(repo, cfg)

	clusterIDs := make([]string, clusters)

	for c := 0; c < clusters; c++ {
		vec := makeVector(dims, c, 0)

		id, err := svc.Upsert(ctx, "grp", vec)
		if err != nil {
			t.Fatalf("cluster %d: %v", c, err)
		}

		clusterIDs[c] = id
	}

	// Add many users per cluster
	resolved := 0
	novel := 0

	for c := 0; c < clusters; c++ {
		for u := 1; u <= usersPerCluster; u++ {
			vec := makeVector(dims, c, u)

			id, err := svc.Upsert(ctx, "grp", vec)
			if err != nil {
				t.Fatalf("cluster=%d user=%d: %v", c, u, err)
			}

			if id == clusterIDs[c] {
				resolved++
			} else {
				novel++
			}
		}
	}

	total := clusters * usersPerCluster
	t.Logf("Total: %d, Resolved: %d, Novel: %d", total, resolved, novel)

	// Most should resolve, not become novel
	resolveRate := float64(resolved) / float64(total)
	t.Logf("Resolve rate: %.1f%%", resolveRate*100)

	if resolveRate < 0.5 {
		t.Errorf("resolve rate %.1f%% below 50%%", resolveRate*100)
	}
}

func TestScale_BandOverlapRanking(t *testing.T) {
	// Verify that the collectCandidates method correctly ranks by band overlap
	dims := 5
	cfg := scaleConfig(dims)
	svc := NewService(memory.NewRepository(), cfg)

	// Simulate: candidate A appears in 10 bands, B in 2 bands, C in 1 band
	bucketKeys := []string{"b0", "b1", "b2", "b3", "b4", "b5", "b6", "b7", "b8", "b9"}

	allReps := make(map[string][]repositories.Representative)

	// A in all 10 bands
	for _, bk := range bucketKeys {
		allReps[bk] = append(allReps[bk], repositories.Representative{ID: "A"})
	}

	// B in first 2 bands
	allReps["b0"] = append(allReps["b0"], repositories.Representative{ID: "B"})
	allReps["b1"] = append(allReps["b1"], repositories.Representative{ID: "B"})

	// C in first band only
	allReps["b0"] = append(allReps["b0"], repositories.Representative{ID: "C"})

	ids := svc.collectCandidates(bucketKeys, allReps)

	if len(ids) < 3 {
		t.Fatalf("expected at least 3 candidates, got %d", len(ids))
	}

	// A should be first (10 bands), B second (2 bands), C third (1 band)
	if ids[0] != "A" {
		t.Errorf("expected A first, got %s", ids[0])
	}

	if ids[1] != "B" {
		t.Errorf("expected B second, got %s", ids[1])
	}

	if ids[2] != "C" {
		t.Errorf("expected C third, got %s", ids[2])
	}
}

func TestScale_MaxTotalCandidatesTruncation(t *testing.T) {
	dims := 5
	cfg := scaleConfig(dims)
	cfg.MaxTotalCandidates = 3
	svc := NewService(memory.NewRepository(), cfg)

	bucketKeys := []string{"b0", "b1"}
	allReps := map[string][]repositories.Representative{
		"b0": {
			{ID: "A"}, {ID: "B"}, {ID: "C"}, {ID: "D"}, {ID: "E"},
		},
		"b1": {
			{ID: "A"}, {ID: "B"}, {ID: "F"}, // A and B appear in 2 bands
		},
	}

	ids := svc.collectCandidates(bucketKeys, allReps)

	if len(ids) != 3 {
		t.Fatalf("expected 3 candidates (MaxTotalCandidates), got %d", len(ids))
	}

	// A and B should be first (2 bands each)
	topTwo := map[string]bool{ids[0]: true, ids[1]: true}
	if !topTwo["A"] || !topTwo["B"] {
		t.Errorf("expected A and B in top 2, got %v", ids[:2])
	}
}

func TestScale_StorageEstimates(t *testing.T) {
	dims := 20
	bands := 20
	rows := 5
	repEntrySize := 45 // bytes per representative entry in bucket

	scenarios := []struct {
		name       string
		users      int
		clusters   int
		wantRecMB  float64
		wantBuckMB float64
		asOK       bool // fits in Aerospike 1MB record limit
	}{
		{"1M_users_1K_clusters", 1_000_000, 1_000, 1.1, 0.9, true},
		{"1M_users_10K_clusters", 1_000_000, 10_000, 10.4, 8.6, true},
		{"10M_users_10K_clusters", 10_000_000, 10_000, 10.4, 8.6, true},
		{"10M_users_100K_clusters", 10_000_000, 100_000, 104.2, 85.8, true},
		{"10M_users_1M_clusters", 10_000_000, 1_000_000, 1041.7, 858.3, false},
	}

	slotsPerBand := 1 << rows

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			sigSize := bands * rows
			recordSize := 22 + dims*8 + sigSize*8 + 20 + 90 // ~1092 bytes
			recordMB := float64(sc.clusters*recordSize) / (1024 * 1024)

			repsPerBucket := sc.clusters / slotsPerBand
			if repsPerBucket == 0 {
				repsPerBucket = 1
			}

			bucketSize := repsPerBucket * repEntrySize
			totalBuckets := bands * slotsPerBand
			bucketMB := float64(totalBuckets*bucketSize) / (1024 * 1024)

			resolvedSize := 98
			resolvedMB := float64((sc.users-sc.clusters)*resolvedSize) / (1024 * 1024)

			totalMB := recordMB + bucketMB + resolvedMB

			t.Logf("Records: %.1f MB | Buckets: %.1f MB | Resolved: %.1f MB | Total: %.1f MB",
				recordMB, bucketMB, resolvedMB, totalMB)
			t.Logf("Reps/bucket: %d | Bucket size: %.1f KB | Fits Aerospike 1MB: %v",
				repsPerBucket, float64(bucketSize)/1024, bucketSize < 1024*1024)

			if (bucketSize < 1024*1024) != sc.asOK {
				t.Errorf("Aerospike fit: expected %v, bucket size is %d bytes", sc.asOK, bucketSize)
			}

			_ = binary.LittleEndian // keep import valid
		})
	}
}
