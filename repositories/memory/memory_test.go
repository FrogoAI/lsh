package memory

import (
	"testing"
)

const (
	missingKey = "missing"
	newValue   = "new"
)

func TestSetRepresentative(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		memberID string
		metadata int64
	}{
		{name: "simple entry", key: "bucket1", memberID: "user1", metadata: 5},
		{name: "empty key", key: "", memberID: "user2", metadata: 0},
		{name: "large metadata", key: "bucket2", memberID: "user3", metadata: 999999},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()

			err := repo.SetRepresentative(tc.key, tc.memberID, tc.metadata)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			m := repo.buckets[tc.key]
			if m == nil {
				t.Fatal("expected bucket to exist")
			}

			meta, ok := m[tc.memberID]
			if !ok || meta != tc.metadata {
				t.Fatalf("got metadata %d (exists=%v), want %d", meta, ok, tc.metadata)
			}
		})
	}
}

func TestSetRepresentative_Idempotent(t *testing.T) {
	repo := NewRepository()

	_ = repo.SetRepresentative("b1", "u1", 10)
	_ = repo.SetRepresentative("b1", "u1", 20)

	m := repo.buckets["b1"]
	if len(m) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m))
	}

	if m["u1"] != 20 {
		t.Fatalf("expected updated metadata 20, got %d", m["u1"])
	}
}

func TestSetRepresentative_MultipleMembers(t *testing.T) {
	repo := NewRepository()

	_ = repo.SetRepresentative("b1", "u1", 5)
	_ = repo.SetRepresentative("b1", "u2", 10)
	_ = repo.SetRepresentative("b1", "u3", 15)

	m := repo.buckets["b1"]
	if len(m) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m))
	}
}

func TestGetRepresentatives(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(r *Repository)
		key     string
		wantLen int
		wantNil bool
	}{
		{name: "missing key", setup: func(_ *Repository) {}, key: "x", wantNil: true},
		{
			name: "single member",
			setup: func(r *Repository) {
				r.buckets["b1"] = map[string]int64{"u1": 3}
			},
			key:     "b1",
			wantLen: 1,
		},
		{
			name: "multiple members",
			setup: func(r *Repository) {
				r.buckets["b2"] = map[string]int64{"u1": 1, "u2": 2}
			},
			key:     "b2",
			wantLen: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()
			tc.setup(repo)

			reps, err := repo.GetRepresentatives(tc.key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantNil {
				if reps != nil {
					t.Fatalf("expected nil, got %v", reps)
				}

				return
			}

			if len(reps) != tc.wantLen {
				t.Fatalf("expected %d reps, got %d", tc.wantLen, len(reps))
			}
		})
	}
}

func TestSaveAndGetRecords(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(r *Repository)
		keys     []string
		wantLen  int
		wantKey  string
		wantBin  string
		wantBinV any
	}{
		{
			name: "single record",
			setup: func(r *Repository) {
				_ = r.SaveRecord("r1", map[string]any{"name": "alice"})
			},
			keys: []string{"r1"}, wantLen: 1, wantKey: "r1", wantBin: "name", wantBinV: "alice",
		},
		{
			name:  "missing key returns nothing",
			setup: func(_ *Repository) {},
			keys:  []string{missingKey}, wantLen: 0,
		},
		{
			name:  "empty keys slice",
			setup: func(_ *Repository) {},
			keys:  []string{}, wantLen: 0,
		},
		{
			name:  "nil keys slice",
			setup: func(_ *Repository) {},
			keys:  nil, wantLen: 0,
		},
		{
			name: "mixed existing and missing keys",
			setup: func(r *Repository) {
				_ = r.SaveRecord("r1", map[string]any{"x": 1})
			},
			keys: []string{"r1", missingKey}, wantLen: 1, wantKey: "r1",
		},
		{
			name: "overwrite record",
			setup: func(r *Repository) {
				_ = r.SaveRecord("r1", map[string]any{"v": "old"})
				_ = r.SaveRecord("r1", map[string]any{"v": newValue})
			},
			keys: []string{"r1"}, wantLen: 1, wantKey: "r1", wantBin: "v", wantBinV: newValue,
		},
		{
			name: "empty bins",
			setup: func(r *Repository) {
				_ = r.SaveRecord("r1", map[string]any{})
			},
			keys: []string{"r1"}, wantLen: 1, wantKey: "r1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()
			tc.setup(repo)

			recs, err := repo.GetRecords(tc.keys)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(recs) != tc.wantLen {
				t.Fatalf("expected %d records, got %d", tc.wantLen, len(recs))
			}

			if tc.wantLen > 0 && tc.wantKey != "" {
				if recs[0].Key != tc.wantKey {
					t.Errorf("expected key %s, got %s", tc.wantKey, recs[0].Key)
				}

				if tc.wantBin != "" && recs[0].Bins[tc.wantBin] != tc.wantBinV {
					t.Errorf("expected bin %s=%v, got %v", tc.wantBin, tc.wantBinV, recs[0].Bins[tc.wantBin])
				}
			}
		})
	}
}

func TestPutAndGetValue(t *testing.T) {
	cases := []struct {
		name  string
		setup func(r *Repository)
		key   string
		want  string
	}{
		{
			name:  "simple put and get",
			setup: func(r *Repository) { _ = r.PutValue("k1", "v1") },
			key:   "k1", want: "v1",
		},
		{
			name:  "missing key returns empty",
			setup: func(_ *Repository) {},
			key:   missingKey, want: "",
		},
		{
			name:  "empty key",
			setup: func(r *Repository) { _ = r.PutValue("", "val") },
			key:   "", want: "val",
		},
		{
			name:  "empty value",
			setup: func(r *Repository) { _ = r.PutValue("k", "") },
			key:   "k", want: "",
		},
		{
			name: "overwrite value",
			setup: func(r *Repository) {
				_ = r.PutValue("k", "old")
				_ = r.PutValue("k", newValue)
			},
			key: "k", want: newValue,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()
			tc.setup(repo)

			got, err := repo.GetValue(tc.key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBatchSetAndGet(t *testing.T) {
	repo := NewRepository()

	err := repo.BatchSetRepresentative([]string{"k1", "k2"}, "user1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := repo.BatchGetRepresentatives([]string{"k1", "k2", "k3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result["k1"]) != 1 {
		t.Errorf("expected 1 rep in k1, got %d", len(result["k1"]))
	}

	if len(result["k2"]) != 1 {
		t.Errorf("expected 1 rep in k2, got %d", len(result["k2"]))
	}

	if len(result["k3"]) != 0 {
		t.Errorf("expected 0 reps in k3, got %d", len(result["k3"]))
	}
}

func TestBatchSetRepresentative_Idempotent(t *testing.T) {
	repo := NewRepository()

	_ = repo.BatchSetRepresentative([]string{"k1", "k2"}, "user1", 10)
	_ = repo.BatchSetRepresentative([]string{"k1", "k2"}, "user1", 20)

	result, _ := repo.BatchGetRepresentatives([]string{"k1"})
	if len(result["k1"]) != 1 {
		t.Fatalf("expected 1 rep after re-insert, got %d", len(result["k1"]))
	}

	if result["k1"][0].Metadata != 20 {
		t.Errorf("expected updated metadata 20, got %d", result["k1"][0].Metadata)
	}
}

func TestMaxBucketReps_RejectsNewAtCap(t *testing.T) {
	repo := NewRepository(WithMaxBucketReps(3))

	_ = repo.SetRepresentative("b1", "u1", 1)
	_ = repo.SetRepresentative("b1", "u2", 2)
	_ = repo.SetRepresentative("b1", "u3", 3)
	_ = repo.SetRepresentative("b1", "u4", 4) // should be rejected

	reps, _ := repo.GetRepresentatives("b1")
	if len(reps) != 3 {
		t.Errorf("expected 3 reps (cap), got %d", len(reps))
	}

	ids := make(map[string]bool)
	for _, r := range reps {
		ids[r.ID] = true
	}

	if ids["u4"] {
		t.Error("u4 should have been rejected (at cap)")
	}
}

func TestMaxBucketReps_AllowsUpdateAtCap(t *testing.T) {
	repo := NewRepository(WithMaxBucketReps(2))

	_ = repo.SetRepresentative("b1", "u1", 1)
	_ = repo.SetRepresentative("b1", "u2", 2)
	_ = repo.SetRepresentative("b1", "u1", 99) // update existing, should work

	reps, _ := repo.GetRepresentatives("b1")
	if len(reps) != 2 {
		t.Errorf("expected 2 reps, got %d", len(reps))
	}

	for _, r := range reps {
		if r.ID == "u1" && r.Metadata != 99 {
			t.Errorf("u1 metadata: expected 99, got %d", r.Metadata)
		}
	}
}

func TestMaxBucketReps_BatchRespectsCapPerBucket(t *testing.T) {
	repo := NewRepository(WithMaxBucketReps(2))

	_ = repo.BatchSetRepresentative([]string{"b1", "b2"}, "u1", 1)
	_ = repo.BatchSetRepresentative([]string{"b1", "b2"}, "u2", 2)
	_ = repo.BatchSetRepresentative([]string{"b1", "b2"}, "u3", 3) // rejected

	reps1, _ := repo.GetRepresentatives("b1")
	reps2, _ := repo.GetRepresentatives("b2")

	if len(reps1) != 2 {
		t.Errorf("b1: expected 2 reps, got %d", len(reps1))
	}

	if len(reps2) != 2 {
		t.Errorf("b2: expected 2 reps, got %d", len(reps2))
	}
}

func TestMaxBucketReps_ZeroMeansUnlimited(t *testing.T) {
	repo := NewRepository() // no WithMaxBucketReps = 0 = unlimited

	for i := 0; i < 100; i++ {
		_ = repo.SetRepresentative("b1", "u"+string(rune('A'+i)), int64(i))
	}

	reps, _ := repo.GetRepresentatives("b1")
	if len(reps) != 100 {
		t.Errorf("expected 100 reps (unlimited), got %d", len(reps))
	}
}

func TestMaxBucketReps_IndependentBuckets(t *testing.T) {
	repo := NewRepository(WithMaxBucketReps(2))

	_ = repo.SetRepresentative("b1", "u1", 1)
	_ = repo.SetRepresentative("b1", "u2", 2)
	_ = repo.SetRepresentative("b2", "u3", 3)
	_ = repo.SetRepresentative("b2", "u4", 4)

	reps1, _ := repo.GetRepresentatives("b1")
	reps2, _ := repo.GetRepresentatives("b2")

	if len(reps1) != 2 {
		t.Errorf("b1: expected 2, got %d", len(reps1))
	}

	if len(reps2) != 2 {
		t.Errorf("b2: expected 2, got %d", len(reps2))
	}
}

func TestClose(_ *testing.T) {
	repo := NewRepository()
	repo.Close()
}
