# LSH Probability Reference Tables

**Purpose**: Developer reference for understanding LSH collision probabilities and parameter tuning.
**Applies to**: `github.com/FrogoAI/lsh/v2` — both `vector` and `dedup` packages.

---

## Critical Constraint: approximate threshold < strict threshold

The **approximate threshold** (S-curve center where P(any band) = 50%) MUST be strictly below the application's **strict threshold** (CosineThreshold / JaccardThreshold). If violated, recall at the strict threshold drops below 50% — the system misses more matches than it finds.

**Both configs enforce this at runtime via `Config.Validate()`:**

```go
cfg, _ := vector.GetConfigFromEnv()
if err := cfg.Validate(); err != nil {
    log.Fatal(err) // "LSH approximate threshold (X) must be below CosineThreshold (Y)"
}
```

| Config | Approximate threshold | Strict threshold | Constraint |
|---|---|---|---|
| Vector B=40, R=5 | -0.15 | 0.7 | -0.15 < 0.7 PASS |
| Dedup B=40, R=5 | 0.478 | 0.6 | 0.478 < 0.6 PASS |
| Dedup B=40, R=8 | 0.644 | 0.6 | 0.644 >= 0.6 **FAIL** (6.5% recall) |

**Rule**: When tuning B and R, always call `Validate()` to catch violations before deployment.

## How to Read These Tables

**P(any band)** = probability that two items with similarity `s` collide in **at least one** of `B` bands. This is the probability that LSH finds the candidate during bucket lookup.

- **High P(any) at threshold** = good recall (catches true matches) -- what we want
- **High P(any) below threshold** = bucket noise (false candidates loaded from Aerospike) -- wastes I/O but filtered by ExactCosine/ExactJaccard
- **Low P(any) at threshold** = false negatives (misses true matches) -- dangerous, creates ID fragmentation

**Expected bands** = `B * P(one band)` = average number of bands where two items collide. Used by `collectCandidates` to rank candidates -- higher overlap = better match.

**Approximate threshold** = similarity value where P(any band) = 50%. The S-curve transitions sharply around this point. **Must be below the application threshold.**

---

## 1. Vector LSH (Cosine Similarity / Random Hyperplane)

### Formula

```
P(agree per bit) = 1 - arccos(s) / pi
P(one band)      = P(agree)^R
P(any of B bands) = 1 - (1 - P(one band))^B
```

Where `s` = cosine similarity, `B` = bands, `R` = rows per band.

### Recommended config: B=40, R=5, CosineThreshold=0.7

| Cosine (s) | P(agree) | P(one band) | P(any band) | Expected bands | Interpretation |
|:---:|:---:|:---:|:---:|:---:|---|
| 0.0 | 0.5000 | 0.03125 | 71.9% | 1.3 | Orthogonal vectors -- still high collision (noise) |
| **0.1** | 0.5319 | 0.04257 | 82.4% | 1.7 | Very different -- bucket noise |
| **0.2** | 0.5642 | 0.05717 | 90.5% | 2.3 | Different -- bucket noise |
| **0.3** | 0.5970 | 0.07584 | 95.7% | 3.0 | Dissimilar -- noise, ranked low by collectCandidates |
| **0.4** | 0.6310 | 0.10003 | 98.5% | 4.0 | Moderate -- noise, ranked low |
| **0.5** | 0.6667 | 0.13172 | 99.6% | 5.3 | Moderate similarity -- almost always a candidate |
| **0.6** | 0.7048 | 0.17391 | 99.95% | 7.0 | Similar -- verified by ExactCosine |
| **0.7** | **0.7468** | **0.23222** | **99.997%** | **9.3** | **THRESHOLD -- virtually zero miss** |
| **0.8** | 0.7952 | 0.31804 | ~100% | 12.7 | Very similar -- always found |
| **0.9** | 0.8565 | 0.46093 | ~100% | 18.4 | Nearly identical -- always found |
| **1.0** | 1.0000 | 1.00000 | 100% | 40.0 | Identical vectors |

**Approximate threshold** (P(any)=50%): s ~ -0.15 (below zero -- irrelevant for behavioral vectors which always have positive cosine).

**Key insight**: At CosineThreshold=0.7, recall is **99.997%** (miss rate 0.003%). Noise below threshold is filtered by `collectCandidates` band overlap ranking -- true matches have ~9 bands in common, noise has 1-3.

### Alternative configs for comparison

#### B=40, R=3 (wider net, more noise)

| Cosine (s) | P(one band) | P(any band) | Expected bands |
|:---:|:---:|:---:|:---:|
| 0.0 | 0.12500 | 99.5% | 5.0 |
| 0.1 | 0.15059 | 99.8% | 6.0 |
| 0.2 | 0.17969 | 99.96% | 7.2 |
| 0.3 | 0.21280 | 99.99% | 8.5 |
| 0.4 | 0.25119 | ~100% | 10.0 |
| 0.5 | 0.29630 | ~100% | 11.9 |
| 0.6 | 0.35012 | ~100% | 14.0 |
| **0.7** | **0.41646** | **~100%** | **16.7** |
| 0.8 | 0.50294 | ~100% | 20.1 |
| 0.9 | 0.62853 | ~100% | 25.1 |

Wider net: even orthogonal vectors (s=0) are candidates 99.5% of the time. Massive bucket noise. Not recommended.

#### B=40, R=8 (tighter bands, less noise but lower recall)

| Cosine (s) | P(one band) | P(any band) | Expected bands |
|:---:|:---:|:---:|:---:|
| 0.0 | 0.00391 | 14.5% | 0.2 |
| 0.1 | 0.00580 | 20.8% | 0.2 |
| 0.2 | 0.00851 | 29.0% | 0.3 |
| 0.3 | 0.01243 | 39.5% | 0.5 |
| 0.4 | 0.01811 | 51.8% | 0.7 |
| 0.5 | 0.02636 | 65.6% | 1.1 |
| 0.6 | 0.03849 | 79.0% | 1.5 |
| **0.7** | **0.05668** | **90.2%** | **2.3** |
| 0.8 | 0.08495 | 97.0% | 3.4 |
| 0.9 | 0.13543 | 99.7% | 5.4 |

Much less noise (14.5% at s=0 vs 71.9%), but **recall at threshold drops to 90.2%** -- 10% of similar vectors missed. **Not recommended for fraud detection.**

#### B=25, R=8 (fewer bands + tighter = worst recall)

| Cosine (s) | P(one band) | P(any band) | Expected bands |
|:---:|:---:|:---:|:---:|
| 0.0 | 0.00391 | 9.3% | 0.1 |
| 0.1 | 0.00580 | 13.5% | 0.1 |
| 0.2 | 0.00851 | 19.2% | 0.2 |
| 0.3 | 0.01243 | 26.9% | 0.3 |
| 0.4 | 0.01811 | 36.9% | 0.5 |
| 0.5 | 0.02636 | 49.1% | 0.7 |
| 0.6 | 0.03849 | 62.5% | 1.0 |
| **0.7** | **0.05668** | **76.7%** | **1.4** |
| 0.8 | 0.08495 | 89.5% | 2.1 |
| 0.9 | 0.13543 | 97.8% | 3.4 |

**Recall at threshold = 76.7%** -- 23% miss rate. **Completely unacceptable.** Shows why reducing B is dangerous.

### Vector config recommendation

```env
# RECOMMENDED -- do not change B and R
VLSH_BANDS=40
VLSH_ROWS=5
VLSH_MAX_BUCKET_SIZE=1000        # increased from 200 to reduce trimming
VLSH_MAX_TOTAL_CANDIDATES=100
VLSH_VECTOR_DIMENSIONS=20
VLSH_COS_THRESHOLD=0.7
VLSH_SEED=13374269
VLSH_RESOLVED_CACHE_SIZE=500000
```

**Constraint check**: approximate threshold (-0.15) < CosineThreshold (0.7) -- PASS.

---

## 2. Dedup LSH (Jaccard Similarity / MinHash)

### Formula

```
P(one band)        = s^R           (direct -- Jaccard estimate equals collision probability)
P(any of B bands)  = 1 - (1 - s^R)^B
Approx threshold   = (1/B)^(1/R)
```

Where `s` = Jaccard similarity, `B` = bands, `R` = rows per band.

### Current config: B=40, R=5, JaccardThreshold=0.6

**Approximate threshold**: (1/40)^(1/5) = 0.478

| Jaccard (s) | P(one band) | P(any band) | Expected bands | Interpretation |
|:---:|:---:|:---:|:---:|---|
| 0.0 | 0.00000 | 0.0% | 0.0 | Completely different -- never collide |
| **0.1** | 0.00001 | 0.04% | 0.0 | Very different -- practically never |
| **0.2** | 0.00032 | 1.3% | 0.01 | Different -- very rare collision |
| **0.3** | 0.00243 | 9.3% | 0.1 | Dissimilar -- occasional collision |
| **0.4** | 0.01024 | 33.7% | 0.4 | Below threshold -- some collisions |
| **0.5** | 0.03125 | 71.9% | 1.3 | Near threshold -- S-curve transition |
| **0.6** | **0.07776** | **96.1%** | **3.1** | **THRESHOLD -- 96% recall** |
| **0.7** | 0.16807 | 99.9% | 6.7 | Above threshold -- virtually always found |
| **0.8** | 0.32768 | ~100% | 13.1 | Very similar -- always found |
| **0.9** | 0.59049 | ~100% | 23.6 | Nearly identical -- always found |
| **1.0** | 1.00000 | 100% | 40.0 | Identical strings |

**Key insight**: At JaccardThreshold=0.6, recall is **96.1%** (miss rate ~4%). This is acceptable for string deduplication -- if the same string is seen again, it has another 96% chance. Over 3 attempts: 1-(0.04)^3 = 99.994%.

**Jaccard vs Cosine at same B/R**: Jaccard LSH has much cleaner buckets -- collision at s=0.3 is only 9.3% (vs 95.7% for cosine). This is because Jaccard has a sharper S-curve with MinHash.

### Alternative configs for comparison

#### B=40, R=3 (wider net)

**Approximate threshold**: (1/40)^(1/3) = 0.292

| Jaccard (s) | P(one band) | P(any band) | Expected bands |
|:---:|:---:|:---:|:---:|
| 0.0 | 0.00000 | 0.0% | 0.0 |
| 0.1 | 0.00100 | 3.9% | 0.04 |
| 0.2 | 0.00800 | 27.4% | 0.3 |
| 0.3 | 0.02700 | 66.7% | 1.1 |
| 0.4 | 0.06400 | 93.2% | 2.6 |
| 0.5 | 0.12500 | 99.5% | 5.0 |
| **0.6** | **0.21600** | **~100%** | **8.6** |
| 0.7 | 0.34300 | ~100% | 13.7 |
| 0.8 | 0.51200 | ~100% | 20.5 |
| 0.9 | 0.72900 | ~100% | 29.2 |

Higher recall at threshold (~100%), but more noise at s=0.3 (67% vs 9%).

#### B=20, R=5 (fewer bands)

**Approximate threshold**: (1/20)^(1/5) = 0.550

| Jaccard (s) | P(one band) | P(any band) | Expected bands |
|:---:|:---:|:---:|:---:|
| 0.0 | 0.00000 | 0.0% | 0.0 |
| 0.1 | 0.00001 | 0.02% | 0.0 |
| 0.2 | 0.00032 | 0.6% | 0.01 |
| 0.3 | 0.00243 | 4.7% | 0.05 |
| 0.4 | 0.01024 | 18.7% | 0.2 |
| 0.5 | 0.03125 | 47.2% | 0.6 |
| **0.6** | **0.07776** | **80.4%** | **1.6** |
| 0.7 | 0.16807 | 97.7% | 3.4 |
| 0.8 | 0.32768 | ~100% | 6.6 |
| 0.9 | 0.59049 | ~100% | 11.8 |

Less noise, but recall at threshold drops to 80.4% -- 20% miss rate.

#### B=40, R=8 (tighter bands)

**Approximate threshold**: (1/40)^(1/8) = 0.644

| Jaccard (s) | P(one band) | P(any band) | Expected bands |
|:---:|:---:|:---:|:---:|
| 0.0 | 0.00000 | 0.0% | 0.0 |
| 0.1 | 0.00000 | 0.0% | 0.0 |
| 0.2 | 0.00000 | 0.01% | 0.0 |
| 0.3 | 0.00001 | 0.03% | 0.0 |
| 0.4 | 0.00007 | 0.26% | 0.003 |
| 0.5 | 0.00039 | 1.6% | 0.02 |
| **0.6** | **0.00168** | **6.5%** | **0.07** |
| 0.7 | 0.00576 | 20.7% | 0.2 |
| 0.8 | 0.01678 | 49.1% | 0.7 |
| 0.9 | 0.04305 | 83.0% | 1.7 |

Very clean buckets, but recall at threshold = only 6.5%. **Useless for dedup.** Approximate threshold 0.644 >= JaccardThreshold 0.6 — **fails `Validate()`**.

### Dedup config recommendation

```env
# CURRENT DEFAULTS -- validated in production
LSH_BANDS=40
LSH_ROWS=5
LSH_SHINGLE_SIZE=3
LSH_JAC_THRESHOLD=0.6
LSH_MAX_BUCKET_SIZE=200
LSH_MAX_TOTAL_CANDIDATES=100
LSH_SEED=13374269
LSH_RESOLVED_CACHE_SIZE=500000
```

**Constraint check**: approximate threshold (0.478) < JaccardThreshold (0.6) -- PASS.

---

## 3. Side-by-Side: Cosine vs Jaccard at B=40, R=5

| Similarity | Cosine P(any) | Jaccard P(any) | Cosine bands | Jaccard bands |
|:---:|:---:|:---:|:---:|:---:|
| 0.0 | 71.9% | 0.0% | 1.3 | 0.0 |
| 0.1 | 82.4% | 0.04% | 1.7 | 0.0 |
| 0.2 | 90.5% | 1.3% | 2.3 | 0.01 |
| 0.3 | 95.7% | 9.3% | 3.0 | 0.1 |
| 0.4 | 98.5% | 33.7% | 4.0 | 0.4 |
| 0.5 | 99.6% | 71.9% | 5.3 | 1.3 |
| 0.6 | 99.95% | 96.1% | 7.0 | 3.1 |
| 0.7 | 99.997% | 99.9% | 9.3 | 6.7 |
| 0.8 | ~100% | ~100% | 12.7 | 13.1 |
| 0.9 | ~100% | ~100% | 18.4 | 23.6 |
| 1.0 | 100% | 100% | 40.0 | 40.0 |

### Key difference

**Jaccard LSH has a sharper S-curve** -- very clean buckets below threshold (9.3% noise at s=0.3) but slightly lower recall at threshold (96.1% vs 99.997%).

**Cosine LSH has a gradual S-curve** -- noisy buckets even at low similarity (95.7% at s=0.3!) but excellent recall at threshold (99.997%). The noise is handled by `collectCandidates` band overlap ranking.

This is a fundamental property of the two hash families:
- **MinHash** (Jaccard): P(collision) = s, linear relationship, sharp banding S-curve
- **Random Hyperplane** (Cosine): P(collision) = 1-arccos(s)/pi, compressed relationship, gradual S-curve

---

## 4. Parameter Glossary

| Parameter | What it controls | Increase effect | Decrease effect |
|---|---|---|---|
| **Bands (B)** | Number of independent hash groups | More recall, more noise, more bucket entries per vector | Less recall (dangerous!), less noise |
| **Rows (R)** | Hash functions per band | Tighter matching per band, less noise, less recall | Wider matching, more noise, more recall |
| **B x R** | Total signature size | Larger signatures, more memory/storage per record | Smaller signatures |
| **MaxBucketSize** | Cap on entries per bucket | Less trimming, larger buckets in Aerospike | More trimming, ID fragmentation risk |
| **MaxTotalCandidates** | Cap on candidates to verify | More verification work, higher recall in edge cases | Less work, might miss matches ranked 101+ |
| **CosineThreshold** | Minimum similarity to match | Fewer matches (tighter clusters) | More matches (looser clusters) |
| **JaccardThreshold** | Minimum similarity to match | Fewer matches (stricter dedup) | More matches (looser dedup) |
| **Seed** | Random number generator seed | Deterministic hyperplanes/permutations | Different hash functions |

### Rules of thumb

1. **Never reduce B below 40** for fraud detection -- recall is not negotiable
2. **R=5 is the sweet spot** for B=40 -- gives 99.997% recall at cosine 0.7 and 96.1% at Jaccard 0.6
3. **MaxBucketSize** should be 5-10x the expected cluster size. For behavioral vectors with max ~200 users per pattern, 1000 is safe.
4. **MaxTotalCandidates=100** is sufficient -- true matches have 9+ band overlap and always rank in top 100
5. **Changing B or R invalidates all buckets** (config version prefix changes). Plan for bucket rebuild after config change.
