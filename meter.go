package lsh

import "go.opentelemetry.io/otel/attribute"

// Metric names for OpenTelemetry instrumentation.
const (
	MetricUpsertDuration     = "lsh.upsert.duration"      // Histogram: seconds per Upsert call
	MetricUpsertTotal        = "lsh.upsert.total"         // Counter: total Upsert calls by result
	MetricNewIDTotal         = "lsh.new_id.total"         // Counter: new behavioural IDs created
	MetricCandidateCount     = "lsh.candidates.count"     // Histogram: candidates evaluated per Upsert
	MetricExactCompareCount  = "lsh.exact_compare.count"  // Histogram: exact similarity checks per Upsert
	MetricResolvedCacheSize  = "lsh.resolved_cache.size"  // Gauge: current L2 cache entries
	MetricBucketRepsReturned = "lsh.bucket_reps.returned" // Histogram: total reps returned per query
)

// Attribute keys for metric labels.
var (
	AttrResult = attribute.Key("result") // l1_hit, l2_hit, l3_hit, match, new
	AttrGroup  = attribute.Key("group")
)

// Result label values.
const (
	ResultL1Hit = "l1_hit" // Exact record found
	ResultL2Hit = "l2_hit" // In-memory resolved cache hit
	ResultL3Hit = "l3_hit" // Persisted resolved cache hit
	ResultMatch = "match"  // LSH candidate matched
	ResultNew   = "new"    // Novel item, new ID created
)
