package lsh

import (
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestNewInstruments(t *testing.T) {
	m := noop.NewMeterProvider().Meter("test")

	inst, err := NewInstruments(m, "lsh.test.")
	if err != nil {
		t.Fatalf("NewInstruments: %v", err)
	}

	if inst == nil {
		t.Fatal("expected non-nil instruments")
	}

	if inst.UpsertDuration == nil {
		t.Error("UpsertDuration is nil")
	}

	if inst.UpsertTotal == nil {
		t.Error("UpsertTotal is nil")
	}

	if inst.NewIDTotal == nil {
		t.Error("NewIDTotal is nil")
	}

	if inst.CandidateCount == nil {
		t.Error("CandidateCount is nil")
	}

	if inst.ExactCompareCount == nil {
		t.Error("ExactCompareCount is nil")
	}

	if inst.BucketRepsReturned == nil {
		t.Error("BucketRepsReturned is nil")
	}
}

func TestNewInstruments_DifferentPrefixes(t *testing.T) {
	m := noop.NewMeterProvider().Meter("test")

	inst1, err := NewInstruments(m, "lsh.vector.")
	if err != nil {
		t.Fatalf("vector: %v", err)
	}

	inst2, err := NewInstruments(m, "lsh.dedup.")
	if err != nil {
		t.Fatalf("dedup: %v", err)
	}

	if inst1 == inst2 {
		t.Error("different prefixes should produce different instrument instances")
	}
}
