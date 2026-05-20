# lsh/v2

Go library for **Locality-Sensitive Hashing** with two built-in use cases:

- **String deduplication** — MinHash signatures + Jaccard similarity
- **Vector behavioural ID** — random hyperplane signatures + cosine similarity

Both share a common core (banding, pooling, sharding) and a pluggable storage layer.

## Architecture

```
lsh (root)              Core toolkit: banding, signature pools, lock sharding, prefix hashing
├── dedup/              String dedup use case (MinHash + Jaccard)
├── vector/             Vector behavioural ID use case (Hyperplane + Cosine)
└── repositories/       Storage interface + implementations
    ├── memory/         In-memory (for tests)
    └── aerospike/      Aerospike (for production)
```

The core package has **zero dependency on repositories** — it provides pure functions only.
Each use case owns its config, hasher, similarity functions, and record serialization.

## Installation

```sh
go get github.com/FrogoAI/lsh/v2
```

## Usage

### String deduplication

Detects near-duplicate text. The flow: text -> shingles -> MinHash signature -> LSH bands -> representative lookup -> Jaccard verification.

```go
import (
    "context"

    "github.com/FrogoAI/lsh/v2/dedup"
    "github.com/FrogoAI/lsh/v2/repositories/memory"
)

repo := memory.NewRepository()

cfg := &dedup.Config{}
cfg.Bands = 40
cfg.Rows = 5
cfg.ShingleSize = 3
cfg.JaccardThreshold = 0.6
cfg.MaxTotalCandidates = 100
cfg.Seed = 13374269

svc := dedup.NewService(repo, cfg)

id1, _ := svc.Upsert(context.Background(), "group1", "the quick brown fox")
id2, _ := svc.Upsert(context.Background(), "group1", "the quick brown fox jumps")

// id1 == id2 if Jaccard similarity >= threshold
```

`Upsert` returns the group-scoped ID of the matching existing record if a near-duplicate is found, or a new deterministic group-scoped ID if the input is novel. `GetNewID` remains content-only.

### Vector behavioural ID

Assigns a behavioural cluster ID to a float64 feature vector using cosine similarity.

```go
import (
    "context"

    "github.com/FrogoAI/lsh/v2/vector"
    "github.com/FrogoAI/lsh/v2/repositories/memory"
)

repo := memory.NewRepository()

cfg := &vector.Config{}
cfg.Bands = 40
cfg.Rows = 5
cfg.VectorDimensions = 20
cfg.CosineThreshold = 0.5
cfg.MaxTotalCandidates = 100
cfg.Seed = 13374269

svc := vector.NewService(repo, cfg)

// Two similar behavioral vectors get the same ID
id1, _ := svc.Upsert(context.Background(), "org1", bonusHunter1Vec)
id2, _ := svc.Upsert(context.Background(), "org1", bonusHunter2Vec)

// id1 == id2 if cosine similarity >= threshold
```

## Configuration

Both use cases read config from environment variables via [caarlos0/env](https://github.com/caarlos0/env).

### dedup (prefix `LSH`)

| Variable | Default | Description |
|---|---|---|
| `LSH_BANDS` | 40 | Number of LSH bands |
| `LSH_ROWS` | 5 | Rows per band |
| `LSH_SHINGLE_SIZE` | 3 | Character n-gram size |
| `LSH_JAC_THRESHOLD` | 0.6 | Jaccard similarity threshold |
| `LSH_MAX_TOTAL_CANDIDATES` | 100 | Max candidates to evaluate |
| `LSH_SEED` | 13374269 | RNG seed for hash functions |

```go
cfg, err := dedup.GetConfigFromEnv()
```

### vector (prefix `VLSH`)

| Variable | Default | Description |
|---|---|---|
| `VLSH_BANDS` | 40 | Number of LSH bands |
| `VLSH_ROWS` | 5 | Rows per band |
| `VLSH_VECTOR_DIMENSIONS` | 20 | Expected vector dimensionality |
| `VLSH_COS_THRESHOLD` | 0.5 | Cosine similarity threshold |
| `VLSH_MAX_TOTAL_CANDIDATES` | 100 | Max candidates to evaluate |
| `VLSH_SEED` | 13374269 | RNG seed for hyperplanes |

```go
cfg, err := vector.GetConfigFromEnv()
```

## Storage backends

Implement the `repositories.Storage` interface to plug in any backend:

```go
type Storage interface {
    SetRepresentative(bucketKey, memberID string, metadata int64) error
    BatchSetRepresentative(bucketKeys []string, memberID string, metadata int64) error
    GetRepresentatives(bucketKey string) ([]Representative, error)
    BatchGetRepresentatives(bucketKeys []string) (map[string][]Representative, error)
    SaveRecord(key string, bins map[string]any) error
    GetRecords(keys []string) ([]Record, error)
    PutValue(key, value string) error
    GetValue(key string) (string, error)
    Close()
}
```

Built-in implementations:
- `repositories/memory` — in-memory (tests and prototyping)
- `repositories/aerospike` — Aerospike (production)

## How it works

### Overview

```
Input (text or vector)
        |
  1. Deterministic content ID (SHA256) + group scope
        |
  2. L1 cache: exact record lookup --> HIT: return ID
        |
  3. L2 cache: resolved ID lookup --> HIT: return resolved ID
        |
  4. Compute LSH signature (MinHash or Hyperplane)
        |
  5. Split into B bands of R rows --> B bucket keys
        |
  6. Fetch representatives from matching buckets (bounded, small)
        |
  7. Compare input vs representatives:
     - Estimated similarity (fast, signature-based)
     - Exact similarity (precise, on full data)
        |
  8a. MATCH: cache resolution, return existing ID
  8b. NO MATCH: save as new record + add as representative to buckets
```

### Step by step

1. **Deterministic ID** — The input is hashed (SHA256, truncated to 128 bits, base64url-encoded) to produce the content-only `GetNewID` value. `Upsert` then combines that value with the group to produce the stored `memberID`. Same input in the same group produces the same returned ID; same input in different groups produces different returned IDs.

2. **L1 cache (record lookup)** — If a record with this ID already exists in storage, return it immediately. This handles exact duplicate inputs with zero LSH overhead.

3. **L2 cache (resolved lookup)** — If this ID was previously resolved to another ID (i.e., it matched a representative), return the resolved ID. The L2 cache is persisted to storage (`PutValue`), surviving pod restarts.

4. **Signature computation** — The input is converted to a fixed-size signature:
   - *Dedup*: text -> character shingles -> MinHash signature (universal hashing with odd coefficients for implicit 2^64 overflow permutation)
   - *Vector*: float64 vector -> random hyperplane projections -> binary signature (0/1 per hyperplane)

5. **Banding** — The signature (B*R elements) is split into B bands of R rows each. Each band is hashed (xxhash) into a bucket key. This is the core LSH step: similar items collide in at least one band with high probability.

6. **Representative lookup** — Each bucket stores a bounded set of **representatives** (cluster leaders), not all members. This is the key design choice — see [Representative-based storage](#representative-based-storage) below.

7. **Similarity verification** — Candidates are first filtered by estimated similarity (cheap signature comparison), then verified with exact similarity (Jaccard or cosine). This two-stage filter reduces expensive comparisons.

8. **Resolution** — If a match is found, the input's ID is mapped to the representative's ID and cached. If no match, the input becomes a new representative in all its buckets.

### Tuning

The probability of two items colliding in at least one band:

```
Jaccard (MinHash):    P = 1 - (1 - s^R)^B
Cosine (Hyperplane):  P = 1 - (1 - (1 - arccos(s)/pi)^R)^B
```

- **Higher recall** (catch more similar items) -> increase `Bands`
- **Higher precision** (fewer false positives) -> increase `Rows`

Use `Config.CalculateApproximateThreshold()` to see the similarity at which collision probability is ~50% for your settings.

## Representative-based storage

### Problem: hot buckets

The traditional LSH approach stores **all members** in each bucket. This creates two problems:

1. **Unbounded growth** — With N users and only 2^R possible bucket values per band (e.g., 2^5 = 32 for binary hyperplane hashing), the average bucket holds N/32 members. At 1M users that is ~31,000 members per bucket.

2. **Cascade failure** — When buckets exceed a size limit, they get skipped during queries. Items that should match can't find their candidates, become "novel" erroneously, and get added to buckets, making them even bigger.

3. **Storage pressure** — In Aerospike, each bucket is one record with a CDT Map. A bucket with 31K entries (~1.4 MB) may exceed the write-block-size limit, cause write amplification, and transfer excessive data over the network.

### Solution: store only representatives

Each bucket stores **one entry per cluster** (the cluster leader / representative), not one per user. When a new item arrives:

- If it matches a representative -> resolve to that representative's ID (no bucket write)
- If it's novel -> save record, add self as representative to buckets

The bucket uses **set semantics** (keyed by memberID, idempotent upsert). The number of entries per bucket is bounded by the number of distinct clusters, not the number of users.

### Scaling characteristics

| Scenario | Members per bucket (old) | Representatives per bucket (new) |
|---|---|---|
| 1M users, 1K clusters, Rows=5 | ~31,250 | ~31 |
| 1M users, 10K clusters, Rows=5 | ~31,250 | ~312 |
| 1M users, 1K clusters, Rows=10 | ~977 | ~1 |

The representative count grows with the number of **distinct clusters**, not the number of users. This is typically orders of magnitude smaller.

### Memory estimates (1M users, 1K clusters)

| Component | Old (all members) | New (representatives) |
|---|---|---|
| Bucket data | ~2.7 GB (1M x 40 buckets x 68B) | ~2.7 MB (1K x 40 buckets x 68B) |
| Record data | ~2.0 GB | ~2.0 GB (unchanged) |
| Per-query network | ~55 MB (40 x 1.4 MB) | ~55 KB (40 x 1.4 KB) |
| Aerospike record size | ~1.4 MB (may exceed limit) | ~2 KB |

## Candidate ranking (multi-probe)

When querying, the system collects candidates from **all** bands, counts how many bands each candidate appears in, and ranks them by overlap count (descending). The top `MaxTotalCandidates` are selected for record loading and verification.

This is important because:
- Candidates sharing more bands are exponentially more likely to be true matches
- Without ranking, a naive "break after N candidates" approach degrades recall from 99.5% to 23.2% once buckets have >100 representatives
- Ranking preserves full recall (all bands searched) while bounding the expensive verification step

## Scaling analysis (10M users)

### Single record size (Bands=40, Rows=5, Dims=20)

| Component | Bytes |
|---|---|
| Key (base64url SHA256[:16]) | 22 |
| Vector (20 x float64) | 160 |
| Signature (200 x uint64) | 1,600 |
| Group string | ~20 |
| Overhead (headers, bin names) | ~90 |
| **Total** | **~1.9 KB** |

### Storage at scale

| Scenario | Records | Buckets | Resolved cache | Total |
|---|---|---|---|---|
| 1M users, 1K clusters | 1 MB | 1 MB | 93 MB | **95 MB** |
| 1M users, 10K clusters | 10 MB | 9 MB | 93 MB | **112 MB** |
| 10M users, 10K clusters | 10 MB | 9 MB | 934 MB | **953 MB** |
| 10M users, 100K clusters | 104 MB | 86 MB | 925 MB | **1.1 GB** |
| 10M users, 1M clusters | 1 GB | 858 MB | 841 MB | **2.7 GB** |

The dominant cost at 10M users is the **resolved cache** (~93 bytes per resolved user). Records and buckets are proportional to cluster count, not user count.

### Bucket size vs cluster count (Rows=5, 32 slots per band)

| Clusters | Reps/bucket | Bucket size | Fits Aerospike 1MB |
|---|---|---|---|
| 1,000 | 31 | 1.4 KB | yes |
| 10,000 | 312 | 14 KB | yes |
| 100,000 | 3,125 | 137 KB | yes |
| 1,000,000 | 31,250 | 1.4 MB | **no** |

**Aerospike limit**: default `write-block-size` is 1 MB. At >500K distinct clusters, bucket records exceed this. For high-cluster-count use cases, increase `write-block-size` to 8 MB or increase `Rows` (more slots per band).

### Recall vs cluster count

With multi-probe ranking, recall is maintained regardless of cluster count. The system searches all bands and ranks by overlap. `MaxTotalCandidates` limits only the verification step (record loading + similarity check), not band scanning.

| Clusters | MaxTotalCandidates | Recall (cos=0.5) |
|---|---|---|
| 20 | 100 | 100% |
| 100 | 50 | 97% |
| 200 | 20 | 97% |

### Precision guarantee

The algorithm guarantees: if input V is resolved to representative R, then `ExactCosine(V, R.Vector) >= CosineThreshold`. This is enforced by the exact verification step. The estimated similarity filter (signature comparison) uses a margin to avoid false negatives but the final decision is always exact.

## Per-bucket eviction cap

Both repository implementations accept `WithMaxBucketReps(n)` to limit the number of representatives per bucket. This prevents bucket records from exceeding Aerospike's `write-block-size` (default 1 MB) regardless of cluster count.

```go
// Memory (tests)
repo := memory.NewRepository(memory.WithMaxBucketReps(10000))

// Aerospike (production)
repo := aerospike.NewRepository(client, ns, set, aerospike.WithMaxBucketReps(10000))
```

With `MaxBucketReps=10000`: each bucket record stays under **450 KB**, supporting up to **320K clusters** before any eviction. 0 (default) means unlimited.

When a bucket is at cap, new representatives are silently rejected (memory) or oldest entries are trimmed by key order (Aerospike). Existing entries can always be updated.

## API note: userID vs vector-derived ID

`GetNewID` is derived deterministically from the **vector content** (SHA256), not from a user identifier. `Upsert` returns a group-scoped `behavioural_id`: identical vectors in the same group get the same ID, while identical vectors in different groups get different IDs. This is intentional: the ID identifies a **behavior pattern within a group**, not a user.

The consumer (scoring engine) maintains the `user -> behavioural_id` mapping externally by storing the returned ID on the enriched event.

## Known issues and mitigations

### 1. Binary hyperplane bucket cardinality

**Issue**: With Rows=R, each band of the hyperplane hasher produces only 2^R possible bucket values. For R=5, that's 32 buckets per band. All users map into these 32 slots.

**Impact**: Low — with representative-based storage, the bucket size is bounded by cluster count (~31 per bucket for 1K clusters), not user count. The 32-slot limit only matters if you have >32K distinct behavioural clusters.

**Mitigation**: If cluster count is very high, increase `Rows` (e.g., R=10 gives 1024 slots) and compensate recall loss by increasing `Bands`.

### 2. Representative coverage (triangle inequality)

**Issue**: If user C is similar to user B (cos=0.8), and B was resolved to representative A (cos=0.85), but C has cos=0.65 with A (below threshold), then C won't find its cluster and becomes a spurious new representative.

**Impact**: Medium — creates slightly more clusters than ideal. More common at threshold boundaries.

**Mitigation**:
- This naturally self-corrects: C becomes a representative, and future items similar to C will find it.
- Use a lower `CosineThreshold`/`JaccardThreshold` to create looser clusters.
- The two-stage filter (estimated + exact similarity) already uses a margin (`cosineMargin=0.1`, `jaccardMargin=0.1`) to avoid threshold edge effects on the estimated stage.

### 3. Cluster drift over time

**Issue**: The representative is the first item in a cluster. As more items join (resolved to the representative), the cluster's "true center" may drift, but the representative stays fixed.

**Impact**: Low — the representative's threshold defines cluster boundaries. Items near the boundary may flip between clusters across different representative choices.

**Mitigation**:
- Accept that clusters are defined by the representative, not a centroid.
- If precise centroids are needed, maintain them in a separate analytics pipeline.
- Bucket TTL (14 days) naturally refreshes representatives.

### 4. Representative TTL expiration

**Issue**: Buckets have a TTL (14 days in Aerospike). When a bucket expires, its representative is lost. New items that would have matched must re-discover the cluster.

**Impact**: Low — the representative's full record has a longer TTL (90 days). The L2 cache (`PutValue`) also survives bucket expiration. Only truly cold clusters (no queries in 14 days) need re-discovery.

**Mitigation**:
- L2 cache persists resolved mappings with record TTL (90 days).
- Re-discovery is automatic: the first item in a cluster after expiration becomes the new representative.

### 5. MinHash vs Hyperplane bucket distribution

**Issue**: MinHash (dedup) produces full uint64 hash values per row, giving effectively unlimited bucket cardinality. Hyperplane (vector) produces binary 0/1 values, giving only 2^R buckets per band. The same storage interface serves both, but their scaling profiles differ.

**Impact**: Low — representative-based storage handles both cases. Dedup buckets are naturally small; vector buckets are bounded by cluster count.

**Mitigation**: No action needed. The representative model adapts to both distributions.

### 6. Concurrent writes to the same bucket

**Issue**: Two concurrent Upsert calls may both determine they're "novel" and both add themselves as representatives to the same buckets. This is safe (set semantics, idempotent), but may create two representatives for what could be one cluster.

**Impact**: Low — group-level lock sharding (64 mutexes) reduces this. It can happen across different pods.

**Mitigation**:
- Same-group operations are serialized within a process via `groupLocks`.
- Cross-pod: Aerospike MapPutOp is atomic. Both writes succeed, creating two representatives. Future items may match either one. This is acceptable for probabilistic clustering.

## Development

All shortcuts are in the Makefile:

```sh
make test                # Unit tests (no external deps)
make test-integration    # Integration tests (requires Aerospike)
make lint                # golangci-lint
make coverage            # Unit coverage + threshold check (>=70%)
make bench               # Run benchmarks (starts Aerospike automatically)
make bench-baseline      # Rebuild tracked baseline
make bench-compare       # Compare current vs baseline via benchstat
make aerospike-start     # Start Aerospike via podman
make aerospike-stop      # Stop Aerospike
make ci                  # lint + coverage
```

## License

See [LICENSE](LICENSE) for details.
