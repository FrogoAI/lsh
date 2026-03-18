package lsh

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metric name suffixes. Each use case prepends its own prefix: "lsh.vector." or "lsh.dedup.".
const (
	metricUpsertDuration     = "upsert.duration"
	metricUpsertTotal        = "upsert.total"
	metricNewIDTotal         = "new_id.total"
	metricCandidateCount     = "candidates.count"
	metricExactCompareCount  = "exact_compare.count"
	metricBucketRepsReturned = "bucket_reps.returned"
)

// Attribute keys for metric labels.
var (
	AttrResult = attribute.Key("result")
	AttrGroup  = attribute.Key("group")
)

// Result label values.
const (
	ResultL1Hit = "l1_hit"
	ResultL2Hit = "l2_hit"
	ResultL3Hit = "l3_hit"
	ResultMatch = "match"
	ResultNew   = "new"
)

// Instruments holds pre-built OpenTelemetry instruments for an LSH service.
// Created once via NewInstruments and injected into the service.
// All fields are safe for concurrent use (otel instruments are thread-safe).
type Instruments struct {
	UpsertDuration     metric.Float64Histogram
	UpsertTotal        metric.Int64Counter
	NewIDTotal         metric.Int64Counter
	CandidateCount     metric.Int64Histogram
	ExactCompareCount  metric.Int64Histogram
	BucketRepsReturned metric.Int64Histogram
}

// NewInstruments creates all LSH instruments from the given meter.
// Prefix should be "lsh.vector." or "lsh.dedup." to differentiate algorithms.
func NewInstruments(m metric.Meter, prefix string) (*Instruments, error) {
	var (
		inst Instruments
		err  error
	)

	inst.UpsertDuration, err = m.Float64Histogram(prefix+metricUpsertDuration, metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	inst.UpsertTotal, err = m.Int64Counter(prefix + metricUpsertTotal)
	if err != nil {
		return nil, err
	}

	inst.NewIDTotal, err = m.Int64Counter(prefix + metricNewIDTotal)
	if err != nil {
		return nil, err
	}

	inst.CandidateCount, err = m.Int64Histogram(prefix + metricCandidateCount)
	if err != nil {
		return nil, err
	}

	inst.ExactCompareCount, err = m.Int64Histogram(prefix + metricExactCompareCount)
	if err != nil {
		return nil, err
	}

	inst.BucketRepsReturned, err = m.Int64Histogram(prefix + metricBucketRepsReturned)
	if err != nil {
		return nil, err
	}

	return &inst, nil
}
