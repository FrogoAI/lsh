package lsh

import "go.opentelemetry.io/otel/attribute"

// Metric name suffixes for OpenTelemetry instrumentation.
// Each use case prepends its own prefix: "lsh.vector." or "lsh.dedup.".
const (
	MetricUpsertDuration     = "upsert.duration"      // Histogram: seconds per Upsert call
	MetricUpsertTotal        = "upsert.total"         // Counter: total Upsert calls by result
	MetricNewIDTotal         = "new_id.total"         // Counter: new IDs created
	MetricCandidateCount     = "candidates.count"     // Histogram: candidates evaluated per Upsert
	MetricExactCompareCount  = "exact_compare.count"  // Histogram: exact similarity checks per Upsert
	MetricBucketRepsReturned = "bucket_reps.returned" // Histogram: total reps returned per query
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
