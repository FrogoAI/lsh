package memory

import (
	"testing"

	"github.com/FrogoAI/lsh/model"
)

func TestAddToBucket(t *testing.T) {
	cases := []struct {
		name   string
		key    string
		value  string
		length int
	}{
		{name: "simple entry", key: "bucket1", value: "user1", length: 5},
		{name: "empty key", key: "", value: "user2", length: 0},
		{name: "large length", key: "bucket2", value: "user3", length: 999999},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()

			err := repo.AddToBucket(tc.key, tc.value, tc.length)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(repo.buckets[tc.key]) != 1 || repo.buckets[tc.key][0] != tc.value {
				t.Fatalf("expected bucket[%q] = [%q], got %v", tc.key, tc.value, repo.buckets[tc.key])
			}

			if len(repo.lens[tc.key]) != 1 || repo.lens[tc.key][0] != tc.length {
				t.Fatalf("expected lens[%q] = [%d], got %v", tc.key, tc.length, repo.lens[tc.key])
			}
		})
	}
}

func TestGetBucketMembers(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(r *Repository)
		key         string
		wantIDs     []string
		wantLens    []int
		wantNilIDs  bool
		wantNilLens bool
	}{
		{
			name:        "missing key returns nil",
			setup:       func(_ *Repository) {},
			key:         "nonexistent",
			wantNilIDs:  true,
			wantNilLens: true,
		},
		{
			name: "single member",
			setup: func(r *Repository) {
				r.buckets["b1"] = []string{"u1"}
				r.lens["b1"] = []int{3}
			},
			key:      "b1",
			wantIDs:  []string{"u1"},
			wantLens: []int{3},
		},
		{
			name: "multiple members",
			setup: func(r *Repository) {
				r.buckets["b2"] = []string{"u1", "u2", "u3"}
				r.lens["b2"] = []int{1, 2, 3}
			},
			key:      "b2",
			wantIDs:  []string{"u1", "u2", "u3"},
			wantLens: []int{1, 2, 3},
		},
		{
			name: "returned slices are copies",
			setup: func(r *Repository) {
				r.buckets["b3"] = []string{"orig"}
				r.lens["b3"] = []int{10}
			},
			key:      "b3",
			wantIDs:  []string{"orig"},
			wantLens: []int{10},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()
			tc.setup(repo)

			ids, lens, err := repo.GetBucketMembers(tc.key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantNilIDs {
				if ids != nil {
					t.Fatalf("expected nil ids, got %v", ids)
				}

				return
			}

			if len(ids) != len(tc.wantIDs) {
				t.Fatalf("expected %d ids, got %d", len(tc.wantIDs), len(ids))
			}

			for i := range ids {
				if ids[i] != tc.wantIDs[i] {
					t.Errorf("ids[%d] = %q, want %q", i, ids[i], tc.wantIDs[i])
				}
			}

			for i := range lens {
				if lens[i] != tc.wantLens[i] {
					t.Errorf("lens[%d] = %d, want %d", i, lens[i], tc.wantLens[i])
				}
			}
		})
	}
}

func TestSaveRecord(t *testing.T) {
	cases := []struct {
		name   string
		record model.Record
	}{
		{
			name:   "basic record",
			record: model.Record{ID: "r1", Input: "hello", GroupID: "g1", Signature: []uint64{1, 2}},
		},
		{
			name:   "empty fields",
			record: model.Record{ID: "r2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()

			err := repo.SaveRecord(tc.record)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, ok := repo.profiles[tc.record.ID]
			if !ok {
				t.Fatalf("record %q not found", tc.record.ID)
			}

			if got.ID != tc.record.ID || got.Input != tc.record.Input {
				t.Fatalf("got %+v, want %+v", got, tc.record)
			}
		})
	}
}

func TestGetRecords(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(r *Repository)
		ids     []string
		wantLen int
	}{
		{
			name:    "no ids requested",
			setup:   func(_ *Repository) {},
			ids:     []string{},
			wantLen: 0,
		},
		{
			name: "some ids found",
			setup: func(r *Repository) {
				r.profiles["a"] = model.Record{ID: "a", Input: "x"}
				r.profiles["b"] = model.Record{ID: "b", Input: "y"}
			},
			ids:     []string{"a", "c"},
			wantLen: 1,
		},
		{
			name: "all ids found",
			setup: func(r *Repository) {
				r.profiles["a"] = model.Record{ID: "a"}
				r.profiles["b"] = model.Record{ID: "b"}
			},
			ids:     []string{"a", "b"},
			wantLen: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()
			tc.setup(repo)

			res, err := repo.GetRecords(tc.ids)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(res) != tc.wantLen {
				t.Fatalf("expected %d records, got %d", tc.wantLen, len(res))
			}
		})
	}
}

func TestResolvedID(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(r *Repository)
		bid    string
		wantID string
	}{
		{
			name:   "not found returns empty",
			setup:  func(_ *Repository) {},
			bid:    "missing",
			wantID: "",
		},
		{
			name: "found returns resolved",
			setup: func(r *Repository) {
				r.resolved["abc"] = "xyz"
			},
			bid:    "abc",
			wantID: "xyz",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()
			tc.setup(repo)

			got, err := repo.GetResolvedID(tc.bid)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.wantID {
				t.Fatalf("got %q, want %q", got, tc.wantID)
			}
		})
	}
}

func TestSaveResolvedID(t *testing.T) {
	repo := NewRepository()

	err := repo.SaveResolvedID("bid1", "resolved1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := repo.GetResolvedID("bid1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "resolved1" {
		t.Fatalf("got %q, want %q", got, "resolved1")
	}
}

func TestClose(_ *testing.T) {
	repo := NewRepository()
	repo.Close() // should not panic
}

func TestBatchAddToBuckets(t *testing.T) {
	cases := []struct {
		name   string
		keys   []string
		value  string
		length int
	}{
		{name: "single key", keys: []string{"k1"}, value: "v1", length: 3},
		{name: "multiple keys", keys: []string{"k1", "k2", "k3"}, value: "v1", length: 5},
		{name: "empty keys", keys: []string{}, value: "v1", length: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()

			err := repo.BatchAddToBuckets(tc.keys, tc.value, tc.length)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for _, k := range tc.keys {
				if len(repo.buckets[k]) != 1 || repo.buckets[k][0] != tc.value {
					t.Errorf("bucket[%q] = %v, want [%q]", k, repo.buckets[k], tc.value)
				}
			}
		})
	}
}

func TestBatchGetBuckets(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(r *Repository)
		keys    []string
		wantLen int
	}{
		{
			name:    "empty keys",
			setup:   func(_ *Repository) {},
			keys:    []string{},
			wantLen: 0,
		},
		{
			name: "keys with data",
			setup: func(r *Repository) {
				r.buckets["k1"] = []string{"a", "b"}
				r.lens["k1"] = []int{1, 2}
				r.buckets["k2"] = []string{"c"}
				r.lens["k2"] = []int{3}
			},
			keys:    []string{"k1", "k2"},
			wantLen: 2,
		},
		{
			name:    "missing key returns empty slice",
			setup:   func(_ *Repository) {},
			keys:    []string{"missing"},
			wantLen: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()
			tc.setup(repo)

			buckets, sizes, err := repo.BatchGetBuckets(tc.keys)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(buckets) != tc.wantLen {
				t.Fatalf("expected %d bucket entries, got %d", tc.wantLen, len(buckets))
			}

			if len(sizes) != tc.wantLen {
				t.Fatalf("expected %d size entries, got %d", tc.wantLen, len(sizes))
			}
		})
	}
}
