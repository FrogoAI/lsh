# PO Requirements Clarification: behavioural_id

**Date**: 2026-03-18
**Source**: Product Owner feedback
**Status**: Accepted clarification, updating initiative

---

## 1. Refined Business Requirements

The PO clarifies three distinct requirements:

### 1.1 behavioural_id Assignment

Assign each user a **single `behavioural_id`** based on their product activity. The implementation is not limited to event chains — any approach that groups users by behavioral similarity is acceptable.

**Properties of behavioural_id**:
- One ID per user (not multiple)
- Stable over time (same behavior = same ID)
- Reusable as a scoring feature in other policies

### 1.2 Fraud Pattern Scoring via behavioural_id

Once assigned, `behavioural_id` enables two scoring signals:

| Signal | Logic |
|--------|-------|
| **Confirmed fraud contamination** | If a `behavioural_id` group contains confirmed fraud users, all members are suspicious |
| **Crowd anomaly** | If a `behavioural_id` group has too many users for the pattern type, the group is suspicious |

### 1.3 Affiliate Fraud Detection

Separate task: detect affiliate fraud via `stream_id` analysis.

**Rule**: Within one `stream_id`, if there are **few distinct `behavioural_id` values but many users**, it indicates affiliate fraud — a single source is sending scripted/templated users who all behave identically.

**Query shape**: `COUNT(users) / COUNT(DISTINCT behavioural_id) WHERE stream_id = X` — high ratio = suspicious.

---

## 2. Concrete Fraud Scenarios

### 2.1 Bonus Hunting

```
1. Register from account_geo = AZ
2. Deposit X EUR
3. Activate promocode
4. Next successful deposit = 10X EUR
5. Withdraw at bet_sum / deposit_sum ratio 1.0-1.2
```

**Feature vector mapping**:

| Feature | Expected Value | Source |
|---------|---------------|--------|
| `deposit_count` | Low (2-3) | FixedVariableQuery: count, filter: deposit |
| `withdrawal_count` | 1 | FixedVariableQuery: count, filter: withdrawal |
| `deposit_sum` | ~11X | FixedVariableQuery: sum, filter: deposit |
| `deposit_std` | HIGH | FixedVariableQuery: std, filter: deposit (variance between X and 10X) |
| `deposit_regularity` | LOW | Derived: `1 / (1 + deposit_std / deposit_avg)` |
| `withdrawal_deposit_ratio` | ~1.0 | Derived: `withdrawal_sum / deposit_sum` |
| `bet_to_deposit_ratio` | 1.0-1.2 | Derived: uses `bet_count_casino + bet_count_sport` vs `deposit_sum` |
| `promo_activation_count` | > 0 | **NEW**: FixedVariableQuery: count, filter: `event_type:promo_activation` |
| `unique_cards` | 1 | FixedVariableQuery: distinct, distinctBy: `card.pan_hash` |
| `unique_ips` | 1-2 | FixedVariableQuery: distinct, distinctBy: `input_ip.ip` |

**Note on geo**: `account.geo = AZ` is a categorical feature. Cosine LSH on numeric vectors doesn't handle categoricals natively. Options:
- Ignore geo in the vector, let numeric features be sufficient for clustering
- Add binary indicator: `is_high_risk_geo` (1 if geo in [AZ, ...], 0 otherwise) — extends vector by 1 dimension
- Use `group` parameter: run separate LSH groups per geo (over-segments, not recommended)

**Recommendation**: Add `is_high_risk_geo` as a binary feature. Keeps vector numeric, adds geo signal without dimensionality explosion.

### 2.2 Money Laundering

```
1. Registration
2. Successful deposit X EUR
3. Withdrawal at bet_sum / deposit_sum ratio 1.0-2.0 (X EUR +/- 10%)
4. Successful deposit X EUR
5. Withdrawal at bet_sum / deposit_sum ratio 1.0-2.0 (X EUR +/- 10%)
```

**Feature vector mapping**:

| Feature | Expected Value | Source |
|---------|---------------|--------|
| `deposit_count` | Moderate (repeating cycles) | FixedVariableQuery: count |
| `withdrawal_count` | ~= deposit_count | FixedVariableQuery: count |
| `deposit_avg` | X (consistent) | FixedVariableQuery: average |
| `deposit_std` | VERY LOW (same amount each time) | FixedVariableQuery: std |
| `withdrawal_std` | VERY LOW | FixedVariableQuery: std |
| `deposit_regularity` | ~1.0 (very regular) | Derived: `1 / (1 + deposit_std / deposit_avg)` |
| `withdrawal_regularity` | ~1.0 | Derived |
| `withdrawal_deposit_ratio` | 1.0-2.0 (stable) | Derived |
| `bet_to_deposit_ratio` | VERY LOW (almost no betting) | Derived |
| `unique_cards` | 1 | FixedVariableQuery: distinct |
| `unique_payways` | 1 | FixedVariableQuery: distinct |
| `payout_concentration` | 1.0 (single method) | Derived: `1 / unique_payways` |

This scenario maps **very well** to aggregate features. The signal is in the regularity and ratios, not in the specific event ordering. Two money-laundry users with different absolute amounts (X=100 vs X=500) but same ratios will have **similar normalized vectors** and will receive the same `behavioural_id` from LSH.

---

## 3. LSH as behavioural_id Engine (Corrected Understanding)

### 3.1 How LSH Produces a Single ID

The existing LSH library's `Upsert` method works as a **deduplication engine**:

```
User vector arrives
       |
       v
Hash into B buckets (internal indexing, NOT the output)
       |
       v
Collect candidate records from those buckets
       |
       v
Verify similarity (cosine) against candidates
       |
       v
Match found (similarity >= threshold)?
  YES --> return existing behavioural_id
  NO  --> create new behavioural_id, store record, return it
```

**Key point**: Buckets are an internal lookup mechanism. The user-facing output is always **one `behavioural_id`**. The B=40 bands just increase the probability of finding the right candidate — more bands = higher recall.

### 3.2 Upsert API for Behavioral Vectors

The vector LSH adaptation should **keep the Upsert pattern**, not split into Index + FindSimilar as initially proposed in `lsh.md`:

```go
// Upsert assigns a behavioural_id to the user based on their feature vector.
// If a similar vector already exists, returns its ID. Otherwise creates a new one.
func (s *VectorService) Upsert(
    ctx context.Context,
    group string,       // e.g. org_id
    userID string,      // account.identifier
    vector []float64,   // normalized feature vector
) (string, error)       // returns behavioural_id
```

This is semantically identical to the existing string-dedup `Upsert` — just with `[]float64` instead of `string` as input.

### 3.3 behavioural_id Lifecycle

```
Event 1 (registration):  vector sparse, min_events not met → no behavioural_id yet
Event 2 (deposit):        vector still thin → no behavioural_id
...
Event N (min_events met): vector computed → Upsert → behavioural_id assigned
Event N+1:                vector updated   → Upsert → same or different behavioural_id
```

The `behavioural_id` can change if the user's behavior shifts significantly (vector moves to a different region). This is desirable — a user who starts normal and transitions to fraud should get a new ID.

---

## 4. Hot-Bucket Analysis

### 4.1 What Are Hot Buckets?

With B=40 bands and R=5 rows per band, each vector produces 40 internal bucket keys. A "hot bucket" is one where many users collide — typically the "normal" population whose normalized vectors are similar.

### 4.2 Why the PO Is Concerned

If 95% of users are "normal" with similar vectors:
- They collide into a few mega-buckets (thousands of entries)
- `MaxBucketSize=200` cap truncates membership
- Aerospike map operations on large buckets are expensive
- Storage cost: 40 bands x N users = many entries, most in hot buckets

### 4.3 Does It Break behavioural_id Assignment?

**No, it doesn't break correctness.** Here's why:

When a new normal user arrives:
1. Their vector hashes into hot buckets (same as other normals)
2. `MaxBucketSize=200` cap means they see up to 200 candidates from each bucket
3. They find a similar candidate → get assigned the **existing** `behavioural_id`
4. The Upsert succeeds — the user gets a normal-pattern ID

When a new fraud user arrives:
1. Their vector (unusual ratios, regularity) hashes into **different** buckets than normals
2. Buckets are small (few fraud users) → no hot-bucket problem
3. Either matches an existing fraud pattern → gets that ID
4. Or no match → new `behavioural_id` created

**The fraud path has no hot-bucket problem.** Hot buckets only affect normal users, and they don't break the ID assignment — they just add Aerospike load.

### 4.4 When Hot Buckets DO Cause Issues

| Issue | Trigger | Impact | Severity |
|-------|---------|--------|----------|
| **ID fragmentation** | MaxBucketSize cap + popular pattern → candidate #201 can't find a match → gets NEW id instead of existing | Multiple IDs for same pattern | Medium |
| **Latency** | Large candidate lists → many cosine verifications per Upsert | Higher p99 on scoring hot path | Medium |
| **Storage** | 40 entries per user in bucket maps | Aerospike memory/disk | Low (TTL-managed) |

### 4.5 Mitigation Strategies

| Strategy | How | Effect |
|----------|-----|--------|
| **Increase MaxBucketSize** | Set to 500-1000 for behavioral use case | Reduces fragmentation at cost of more verifications |
| **Representative record** | Each bucket stores one "representative" record (centroid). New users compare against representative, not all members | Constant-time verification, no size cap needed |
| **Fewer bands for normal** | Reduce bands (e.g. B=20) → fewer bucket entries, still enough recall for fraud patterns | Less storage, fewer hot buckets |
| **Early termination** | Stop candidate verification after first match above threshold | Reduces latency for hot buckets without affecting correctness |
| **Bucket TTL** | 14-day TTL auto-expires stale entries | Natural cleanup, prevents unbounded growth |

**Recommended**: Combine representative record + early termination + bucket TTL. This eliminates the hot-bucket problem entirely while preserving `behavioural_id` assignment quality.

### 4.6 Quantitative Estimate

Given: 100K active users, 40 bands, MaxBucketSize=500

| Population | % | Users | Buckets per user | Total entries | Avg bucket size |
|------------|---|-------|------------------|---------------|-----------------|
| Normal | 95% | 95K | 40 | 3.8M | ~200-500 (hot) |
| Suspicious | 4% | 4K | 40 | 160K | ~5-20 (warm) |
| Fraud | 1% | 1K | 40 | 40K | ~2-10 (cold) |

With representative record approach, each bucket stores 1 record regardless of member count → bucket size is always 1 → no hot buckets at all. Member list moves to a separate `user_patterns` record set (key=behavioural_id, bin=member_list).

---

## 5. Affiliate Fraud Detection

### 5.1 Requirement

> Within one `stream_id`, if there are few `behavioural_id` but many users, it's affiliate fraud.

### 5.2 Implementation

Once `behavioural_id` is assigned (via LSH Upsert), it becomes a field on the enriched event. Two new `FixedVariableQuery` variables enable the detection:

```json
[
  {
    "name": "stream_user_count",
    "query_kind": "fixed-variable-query",
    "query": {
      "group_by": ["account.stream_id"],
      "retrieve": "payment_num.amount_eur",
      "function": "count",
      "range": "30d"
    }
  },
  {
    "name": "stream_distinct_patterns",
    "query_kind": "fixed-variable-query",
    "query": {
      "group_by": ["account.stream_id"],
      "distinct_by": "account.behavioural_id",
      "retrieve": "payment_num.amount_eur",
      "function": "distinct",
      "range": "30d"
    }
  }
]
```

**Trigger expression**:
```
stream_user_count > 10 AND stream_user_count / stream_distinct_patterns > 5
```

Meaning: if a stream has >10 users and >5 users per distinct pattern, flag as affiliate fraud.

### 5.3 Pipeline Requirement

For `distinct_by: account.behavioural_id` to work, the `behavioural_id` must be available as a field on the ScoringEvent **before** the affiliate fraud query executes.

**Execution order in policy tree (via DependsOn)**:
```
FixedVariableQueries (deposit_count, etc.)     -- existing features
       |
       v
UserPatternQuery (computes behavioural_id)     -- LSH Upsert
       |
       v
Source.Set("account.behavioural_id", result)   -- enriches event
       |
       v
stream_distinct_patterns query                  -- uses enriched field
       |
       v
Affiliate fraud trigger expression
```

---

## 6. Revised Feature Vector

Based on the PO's scenarios, the feature vector gains two new features:

| # | Feature | Type | Source | New? |
|---|---------|------|--------|------|
| 1 | `deposit_count` | Frequency | FixedVariableQuery | No |
| 2 | `withdrawal_count` | Frequency | FixedVariableQuery | No |
| 3 | `event_count_total` | Frequency | FixedVariableQuery | No |
| 4 | `deposit_sum` | Financial | FixedVariableQuery | No |
| 5 | `withdrawal_sum` | Financial | FixedVariableQuery | No |
| 6 | `deposit_avg` | Financial | FixedVariableQuery | No |
| 7 | `deposit_std` | Financial | FixedVariableQuery | No |
| 8 | `withdrawal_std` | Financial | FixedVariableQuery | No |
| 9 | `deposit_max` | Financial | FixedVariableQuery | No |
| 10 | `withdrawal_deposit_ratio` | Derived | `withdrawal_sum / deposit_sum` | No |
| 11 | `deposit_regularity` | Derived | `1 / (1 + deposit_std / deposit_avg)` | No |
| 12 | `withdrawal_regularity` | Derived | `1 / (1 + withdrawal_std / withdrawal_avg)` | No |
| 13 | `activity_density` | Derived | `event_count_total / 30` | No |
| 14 | `unique_payways` | Cardinality | FixedVariableQuery HLL | No |
| 15 | `unique_cards` | Cardinality | FixedVariableQuery HLL | No |
| 16 | `unique_ips` | Cardinality | FixedVariableQuery HLL | No |
| 17 | `bet_to_deposit_ratio` | Derived | `(bet_count_casino + bet_count_sport) / deposit_count` | No |
| 18 | `payout_concentration` | Derived | `1 / unique_payways` | No |
| 19 | `promo_activation_count` | Frequency | FixedVariableQuery: count, filter: `event_type:promo_activation` | **YES** |
| 20 | `is_high_risk_geo` | Binary | 1 if `account.geo` in configured risk list, else 0 | **YES** |

**Dimensions**: 20 (was 18). Update `VectorDimensions` config default to 20.

---

## 7. Open Questions for PO

| # | Question | Why It Matters |
|---|----------|----------------|
| 1 | **Is ID fragmentation for normal users acceptable?** If two "normal" users get different `behavioural_id` values, does that break scoring or affiliate fraud detection? | Affects MaxBucketSize tuning and whether we need the representative-record optimization |
| 2 | **Which geos are "high risk"?** The PO mentions AZ for bonus hunting. Need the full list for the `is_high_risk_geo` feature, or should it be configurable per org? | Feature engineering |
| 3 | **Is `promo_activation` a distinct event_type?** Or is it embedded in another event (deposit with promocode field)? Need to know correct filter expression. | FixedVariableQuery filter config |
| 4 | **What bet/deposit ratio threshold separates "just enough betting" from "real engagement"?** PO says 1.0-1.2 for bonus hunting. Is this the `bet_sum/deposit_sum` or `bet_count/deposit_count`? | Feature definition precision |
| 5 | **Stream_id availability**: Is `stream_id` always populated for affiliate traffic? Can it be empty for organic users? | Affiliate fraud query guard condition |
| 6 | **How many confirmed fraud users exist?** If we have a list of known fraud user_ids, we can retroactively validate: do they cluster into the same `behavioural_id` groups? | Validation strategy |
