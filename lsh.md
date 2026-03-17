# LSH Adaptation Plan: From String Deduplication to Numeric Vector Clustering

**Date**: 2026-03-17
**Source**: `github.com/FrogoAI/lsh`
**Target**: User behavioral pattern clusterization (user-pattern initiative)

---

## 1. Current Design Summary

The existing LSH library is a **string similarity deduplication service**:

```
Input string ("maxim@weavers.team")
       |
       v
[Shingle] --> character n-grams: {"^ma", "max", "axi", "xim", ...}
       |
       v
[MinHash] --> signature[bands*rows] = min permutation hash per shingle
       |
       v
[Banding] --> bucket keys: "b0:a3f2", "b1:7c1e", ...
       |
       v
[Candidate lookup] --> fetch bucket members, filter by shingle length
       |
       v
[Jaccard verification] --> estimated (signature) then exact (shingle overlap)
       |
       v
Return: existing ID if similar, new ID if unique
```

**Similarity metric**: Jaccard index (set intersection / set union) on character shingle sets.
**Use case**: Deduplication — "is this string similar to one I've seen before?"

---

## 2. What Needs to Change

For user behavioral patterns, the input is a **numeric feature vector**, not a string:

```
Current:  "maxim@weavers.team"  -->  set of character trigrams  -->  Jaccard
Needed:   [30, 28, 0.81, 0.95, 1.2, ...]  -->  numeric vector  -->  Cosine similarity
```

### Changes by layer

| Layer | Current | Needed | Impact |
|-------|---------|--------|--------|
| **Input** | `string` | `[]float64` (feature vector) | API change |
| **Tokenization** | Shingling (char n-grams) | Not needed — input is already structured | Remove |
| **Hash function** | MinHash (set permutation) | Random Hyperplane or SimHash (vector projection) | Core rewrite |
| **Similarity metric** | Jaccard (set overlap) | Cosine similarity (vector angle) | Core rewrite |
| **Length filter** | `minLen/maxLen` shingle count filter | Not applicable for vectors | Remove |
| **Service API** | `Upsert(group, string) -> id` (dedup) | `Index(group, userID, vector)` + `Query(group, vector) -> matches` | API redesign |
| **Record model** | `{ID, Input string, Signature}` | `{ID, Vector []float64, Signature}` | Model change |
| **Bucket storage** | `Map<UserID, ShingleLength>` | `Map<UserID, _>` (no length needed) | Minor |
| **Config** | `ShingleSize`, `JaccardThreshold` | `VectorDimensions`, `CosineThreshold` | Config change |

### What stays the same

| Component | Why it's reusable |
|-----------|-------------------|
| **Banding logic** (`ComputeBands`) | Band/row structure is universal across all LSH variants. Hash bands of signature into bucket keys. |
| **Aerospike connector** | Storage pattern (bucket maps, record bins, batch ops) is identical. Only bin contents change. |
| **Bands/Rows/Seed config** | Same tuning semantics: bands = recall, rows = precision. |
| **MaxBucketSize / MaxTotalCandidates** | Same overflow protection logic. |
| **Signature pool** (`sync.Pool`) | Same allocation optimization for signatures. |
| **Group sharding** (`groupLocks`, `prefixCache`) | Same concurrency pattern. |
| **Config versioning** (`HashVersion`) | Same cache invalidation on config change. |
| **L1/L2 caching** | Reusable for "already-classified user" fast path. |
| **Bucket TTL / Record TTL** | Same expiry semantics. |

---

## 3. Core Algorithm Change: MinHash -> Random Hyperplane LSH

### 3.1 Current: MinHash (Jaccard)

```go
// hasher.go:44 - current
func (h *Hasher) ComputeSignature(tokens set.GenericDataSet[string], sig []uint64) {
    for i := 0; i < h.sigSize; i++ {
        sig[i] = math.MaxUint64
    }
    for token := range tokens {
        hv := xxhash.Sum64String(token)       // hash the token
        for i := 0; i < h.sigSize; i++ {
            ph := h.coefficientA[i]*hv + h.coefficientB[i]  // permutation
            if ph < sig[i] {
                sig[i] = ph                    // keep minimum
            }
        }
    }
}
```

This computes MinHash: for each of `sigSize` random permutations, keep the minimum hash value across all tokens. Two sets with high Jaccard overlap will share minimum values.

### 3.2 Needed: Random Hyperplane LSH (Cosine Similarity)

```
For each hash function i:
    1. Generate a random vector r_i of same dimension as input
    2. Compute dot product: dot = r_i . input_vector
    3. sig[i] = 1 if dot >= 0, else 0
```

Two vectors with high cosine similarity will produce the same sign under most random projections. The fraction of matching bits estimates cosine similarity:

```
cosine_similarity ~ 1 - (hamming_distance / sigSize)
```

### 3.3 Proposed New Hasher

```go
type VectorHasher struct {
    hyperplanes [][]float64  // [sigSize][dimensions] random unit vectors
    bands       int
    rows        int
    sigSize     int
}

func NewVectorHasher(bands, rows, dimensions int, seed int64) *VectorHasher {
    r := rand.New(rand.NewSource(seed))
    sigSize := bands * rows

    hyperplanes := make([][]float64, sigSize)
    for i := 0; i < sigSize; i++ {
        plane := make([]float64, dimensions)
        for d := 0; d < dimensions; d++ {
            plane[d] = r.NormFloat64()  // standard normal distribution
        }
        hyperplanes[i] = plane
    }

    return &VectorHasher{
        hyperplanes: hyperplanes,
        bands:       bands,
        rows:        rows,
        sigSize:     sigSize,
    }
}

// ComputeSignature produces a binary signature from a feature vector.
// Each bit = sign of dot product with a random hyperplane.
func (h *VectorHasher) ComputeSignature(vector []float64, sig []uint64) {
    for i := 0; i < h.sigSize; i++ {
        dot := 0.0
        for d := 0; d < len(vector); d++ {
            dot += h.hyperplanes[i][d] * vector[d]
        }
        if dot >= 0 {
            sig[i] = 1
        } else {
            sig[i] = 0
        }
    }
}
```

### 3.4 ComputeBands — No Change Needed

The existing `ComputeBands` method works unchanged. It takes a `[]uint64` signature and hashes each band of `rows` values into a bucket key. The fact that values are now 0/1 instead of MinHash values doesn't matter — the banding logic is generic.

---

## 4. Similarity Metric Change: Jaccard -> Cosine

### 4.1 Current: Estimated + Exact Jaccard

```go
// Estimated (fast, from signatures)
func EstimateJaccard(sig1, sig2 []uint64) float64 {
    matches := 0
    for i := range sig1 {
        if sig1[i] == sig2[i] { matches++ }
    }
    return float64(matches) / float64(len(sig1))
}

// Exact (slow, from original shingles)
func (s *SimilarityService) CalculateJaccardOptimized(
    sourceSet set.GenericDataSet[string], targetStr string,
) float64 { ... }
```

### 4.2 Needed: Estimated + Exact Cosine

```go
// Estimated (fast, from binary signatures) — Hamming similarity
func EstimateCosine(sig1, sig2 []uint64) float64 {
    if len(sig1) == 0 || len(sig1) != len(sig2) {
        return 0.0
    }
    matches := 0
    for i := range sig1 {
        if sig1[i] == sig2[i] { matches++ }
    }
    // For random hyperplane LSH:
    // P(same sign) = 1 - angle/pi
    // cosine_similarity = cos(pi * (1 - match_fraction))
    matchFraction := float64(matches) / float64(len(sig1))
    return math.Cos(math.Pi * (1 - matchFraction))
}

// Exact (from original vectors)
func CalculateCosineExact(v1, v2 []float64) float64 {
    if len(v1) != len(v2) || len(v1) == 0 {
        return 0.0
    }
    var dot, norm1, norm2 float64
    for i := range v1 {
        dot += v1[i] * v2[i]
        norm1 += v1[i] * v1[i]
        norm2 += v2[i] * v2[i]
    }
    denom := math.Sqrt(norm1) * math.Sqrt(norm2)
    if denom == 0 {
        return 0.0
    }
    return dot / denom
}
```

**Note**: `EstimateJaccard` and `EstimateCosine` have the exact same structure (count matching signature positions). The difference is only in the interpretation:
- Jaccard: match fraction = Jaccard estimate directly
- Cosine: match fraction -> `cos(pi * (1 - fraction))` = cosine estimate

---

## 5. Service API Change

### 5.1 Current: Deduplication-Oriented

```go
func (s *SimilarityService) Upsert(ctx context.Context, group, input string) (string, error)
```

Single method: "insert this string, return existing ID if similar enough, new ID otherwise."

### 5.2 Needed: Clustering-Oriented

Two separate concerns: **indexing** (add user to buckets) and **querying** (find similar users / get bucket members).

```go
// Index adds or updates a user's vector in the LSH index.
// Called per event after feature vector recomputation.
func (s *VectorService) Index(ctx context.Context, group, userID string, vector []float64) error

// FindSimilar returns users with vectors similar to the query vector.
// Uses LSH for candidate generation, then exact cosine for verification.
func (s *VectorService) FindSimilar(
    ctx context.Context, group string, vector []float64, topK int,
) ([]Match, error)

// GetBucketPeers returns all users who share at least one LSH bucket
// with the given vector. Faster than FindSimilar (no verification step).
func (s *VectorService) GetBucketPeers(
    ctx context.Context, group string, vector []float64,
) ([]string, error)

type Match struct {
    UserID     string
    Similarity float64  // exact cosine similarity
}
```

### 5.3 Why Split Index and Query?

In the current string dedup design, every call is both write and query (`Upsert`). For behavioral vectors:
- **Index** runs on every ScoringEvent (high frequency, write path)
- **FindSimilar** runs on-demand or periodically (lower frequency, read path)
- Separation allows different performance tuning per path

---

## 6. Model Change

### 6.1 Current

```go
type Record struct {
    ID        string      // sha256 of input string
    Input     string      // original string (for exact Jaccard)
    GroupID   string
    Signature []uint64    // MinHash signature
}
```

### 6.2 Needed

```go
type Record struct {
    ID        string      // user identifier (account.identifier)
    Vector    []float64   // feature vector (for exact cosine)
    GroupID   string
    Signature []uint64    // hyperplane signature
}
```

**Difference**: `Input string` -> `Vector []float64`. The ID is no longer derived from hashing the input — it's the user identifier from the ScoringEvent.

---

## 7. Storage Change

### 7.1 Aerospike Bins

| Bin | Current | Needed | Change |
|-----|---------|--------|--------|
| `m` (members) | `Map<UserID, ShingleLength>` | `Map<UserID, Timestamp>` | Value semantics change |
| `i` (input) | `string` (original input) | `[]float64` (feature vector) | Type change |
| `g` (group) | `string` | `string` | No change |
| `s` (signature) | `[]uint64` (MinHash) | `[]uint64` (hyperplane bits) | No change in type |
| `r` (resolved) | `string` (resolved bid) | Remove or repurpose | Different caching needs |

### 7.2 Bucket Value Change

Current: `MapPutOp(mapPolicy, binMembers, value, length)` — stores shingle length for pre-filtering.

For vectors, shingle length is meaningless. Options:
- **Option A**: Store `Map<UserID, 1>` — just membership (simplest)
- **Option B**: Store `Map<UserID, timestamp>` — for TTL-based eviction
- **Option C**: Store `Map<UserID, norm>` — vector norm for pre-filtering (analogous to length filter)

**Recommendation**: Option B — timestamp enables tracking when a user's vector was last indexed.

### 7.3 Length Pre-Filter Removal

Current service (`service.go:128-131`):
```go
minLen := int(float64(inputLen) * s.config.JaccardThreshold)
maxLen := int(float64(inputLen) / s.config.JaccardThreshold)
```
This filters candidates by shingle count — strings with very different lengths can't have high Jaccard.

**For vectors**: All vectors have the same dimension. This filter is not needed. Remove.

---

## 8. Config Change

### 8.1 Remove
| Field | Reason |
|-------|--------|
| `ShingleSize` | No shingling for numeric vectors |
| `JaccardThreshold` | Replaced by cosine threshold |

### 8.2 Add
| Field | Purpose | Default |
|-------|---------|---------|
| `VectorDimensions` | Expected feature vector length (for hyperplane generation) | 18 |
| `CosineThreshold` | Minimum cosine similarity to consider a match | 0.7 |

### 8.3 Keep
| Field | Reason |
|-------|--------|
| `Bands` | Same tuning: more bands = higher recall |
| `Rows` | Same tuning: more rows = higher precision |
| `MaxBucketSize` | Same overflow protection |
| `MaxTotalCandidates` | Same candidate cap |
| `Seed` | Same deterministic initialization |

### 8.4 Approximate Threshold Formula

Current: `s ~ (1/B)^(1/R)` — the Jaccard threshold where collision probability transitions from ~0 to ~1.

For cosine LSH: the relationship is different. The probability of two vectors with cosine similarity `s` colliding in one band of `R` rows is:

```
P(collision in one band) = (1 - arccos(s)/pi)^R
P(collision in any band) = 1 - (1 - P(one band))^B
```

The `CalculateApproximateThreshold` method needs update to use this formula.

---

## 9. What About Normalization?

### Problem
Feature vectors have different scales:
- `deposit_count` ~ 0-100
- `deposit_sum` ~ 0-100000 EUR
- `withdrawal_deposit_ratio` ~ 0-1

Raw cosine similarity would be dominated by large-magnitude features.

### Solution: Normalize Before Hashing

**Option A: Z-Score normalization** (recommended)
Each feature is normalized to `(value - mean) / std` using the population statistics from FixedVariableQuery. All features become comparable.

**Option B: Min-Max normalization**
Each feature scaled to [0, 1] using `(value - min) / (max - min)`.

**Where to normalize**: In the estimator, BEFORE calling `lsh.Index()`. The LSH library itself should be normalization-agnostic — it receives a ready vector.

---

## 10. Implementation Plan

### Step 1: Fork or Branch the LSH Module

Create a `v2` or separate package (`vlsh` / `vectorlsh`) to avoid breaking existing string dedup usage.

### Step 2: New Hasher (`hasher.go`)

| Task | Description |
|------|-------------|
| Replace `coefficientA/B []uint64` with `hyperplanes [][]float64` | Random projection vectors instead of permutation coefficients |
| New `NewVectorHasher(bands, rows, dimensions, seed)` | Generate random hyperplanes from normal distribution |
| Replace `ComputeSignature(tokens set, sig)` with `ComputeSignature(vector []float64, sig)` | Dot product + sign instead of min-permutation |
| Keep `ComputeBands(signature)` unchanged | Generic banding logic works for any signature type |

### Step 3: New Similarity Functions (`utils.go`)

| Task | Description |
|------|-------------|
| Replace `Shingle()` | Remove entirely |
| Replace `EstimateJaccard()` with `EstimateCosine()` | Hamming match fraction -> `cos(pi*(1-frac))` |
| Replace `CalculateJaccardOptimized()` with `CalculateCosineExact()` | Dot product / (norm1 * norm2) |

### Step 4: New Service API (`service.go`)

| Task | Description |
|------|-------------|
| Replace `Upsert()` with `Index()` + `FindSimilar()` + `GetBucketPeers()` | Split write and read paths |
| Remove shingle length pre-filter | Not applicable for fixed-dimension vectors |
| Update candidate verification | Use `EstimateCosine` -> `CalculateCosineExact` instead of Jaccard |
| Update Record creation | Store `Vector []float64` instead of `Input string` |

### Step 5: Model + Config (`model/record.go`, `config.go`)

| Task | Description |
|------|-------------|
| `Input string` -> `Vector []float64` in Record | Structural change |
| Remove `ShingleSize`, `JaccardThreshold` from Config | Not applicable |
| Add `VectorDimensions`, `CosineThreshold` to Config | New parameters |
| Update `CalculateApproximateThreshold()` | Cosine formula instead of Jaccard formula |

### Step 6: Storage Adapter (`repositories/aerospike/connector.go`)

| Task | Description |
|------|-------------|
| `binInput` stores `[]float64` instead of `string` | Type change in SaveRecord/GetRecords |
| Bucket value: `length int` -> `timestamp int64` | Semantics change in AddToBucket/BatchAddToBuckets |
| Remove length from `BatchGetBuckets` return | Simplify to `(map[string][]string, error)` |
| Update `GetRecords` deserialization | Parse `[]float64` from Aerospike list bin |

### Step 7: Tests

| Task | Description |
|------|-------------|
| `TestVectorSimilarity` | Two close vectors -> same bucket, high cosine |
| `TestVectorDissimilarity` | Two different vectors -> different buckets, low cosine |
| `TestBandRowTuning` | Verify precision/recall tradeoff with different configs |
| `TestNormalizationInvariance` | Verify that pre-normalized vectors cluster by pattern not magnitude |
| `BenchmarkVectorIndex` | Measure per-event indexing overhead |
| `BenchmarkFindSimilar` | Measure query latency with 10K/100K users |

---

## 11. Effort Estimate Summary

| Component | Files | Complexity |
|-----------|-------|------------|
| VectorHasher (hyperplane core) | `hasher.go` | Medium — new algorithm, but clean replacement |
| Cosine similarity functions | `utils.go` | Low — simple math |
| Service API (Index/FindSimilar) | `service.go` | Medium — split Upsert, remove shingle logic |
| Model change | `model/record.go` | Low — field type change |
| Config change | `config.go` | Low — add/remove fields |
| Aerospike connector | `repositories/aerospike/connector.go` | Medium — bin type changes, deserialization |
| Memory repo (for tests) | `repositories/memory/memory.go` | Low — mirror Aerospike changes |
| Tests | `*_test.go` | Medium — new test scenarios |

**Total**: Achievable in 1 sprint for an engineer familiar with the codebase. The architecture, Aerospike patterns, concurrency model, and banding logic all transfer directly.

---

## 12. Decision: Separate Package or Extend?

### Option A: New Package (`vectorlsh`)
- (+) Zero risk to existing string dedup
- (+) Clean API for the new use case
- (-) Code duplication (banding, Aerospike connector, config versioning)

### Option B: Generic Interface + Two Implementations
- (+) Shared infrastructure (banding, storage, config)
- (+) DRY
- (-) Abstraction overhead, more complex

### Option C: Extend Existing with Feature Flag
- (+) Single package
- (-) Mixed concerns, harder to maintain

**Recommendation**: **Option A** for Phase 2 (get it working fast), then refactor to **Option B** if both variants stay long-term. The string dedup LSH is production-critical — don't risk it by coupling.
