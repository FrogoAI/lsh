package memory

import (
	"testing"

	"github.com/FrogoAI/lsh/repositories"
)

func TestAddBucketMember(t *testing.T) {
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

			err := repo.AddBucketMember(tc.key, tc.memberID, tc.metadata)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			members := repo.buckets[tc.key]
			if len(members) != 1 || members[0].ID != tc.memberID || members[0].Metadata != tc.metadata {
				t.Fatalf("got %+v, want [{ID:%s Metadata:%d}]", members, tc.memberID, tc.metadata)
			}
		})
	}
}

func TestGetBucketMembers(t *testing.T) {
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
				r.buckets["b1"] = []repositories.BucketMember{{ID: "u1", Metadata: 3}}
			},
			key:     "b1",
			wantLen: 1,
		},
		{
			name: "multiple members",
			setup: func(r *Repository) {
				r.buckets["b2"] = []repositories.BucketMember{
					{ID: "u1", Metadata: 1},
					{ID: "u2", Metadata: 2},
				}
			},
			key:     "b2",
			wantLen: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository()
			tc.setup(repo)

			members, err := repo.GetBucketMembers(tc.key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantNil {
				if members != nil {
					t.Fatalf("expected nil, got %v", members)
				}

				return
			}

			if len(members) != tc.wantLen {
				t.Fatalf("expected %d members, got %d", tc.wantLen, len(members))
			}
		})
	}
}

func TestSaveAndGetRecords(t *testing.T) {
	repo := NewRepository()

	err := repo.SaveRecord("r1", map[string]any{"name": "alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs, err := repo.GetRecords([]string{"r1", "missing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}

	if recs[0].Key != "r1" {
		t.Errorf("expected key r1, got %s", recs[0].Key)
	}

	if recs[0].Bins["name"] != "alice" {
		t.Errorf("expected name alice, got %v", recs[0].Bins["name"])
	}
}

func TestPutAndGetValue(t *testing.T) {
	repo := NewRepository()

	err := repo.PutValue("k1", "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, err := repo.GetValue("k1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v != "v1" {
		t.Errorf("expected v1, got %s", v)
	}

	v2, err := repo.GetValue("missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v2 != "" {
		t.Errorf("expected empty string, got %s", v2)
	}
}

func TestBatchAddAndGet(t *testing.T) {
	repo := NewRepository()

	err := repo.BatchAddBucketMember([]string{"k1", "k2"}, "user1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := repo.BatchGetBucketMembers([]string{"k1", "k2", "k3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result["k1"]) != 1 {
		t.Errorf("expected 1 member in k1, got %d", len(result["k1"]))
	}

	if len(result["k2"]) != 1 {
		t.Errorf("expected 1 member in k2, got %d", len(result["k2"]))
	}

	if len(result["k3"]) != 0 {
		t.Errorf("expected 0 members in k3, got %d", len(result["k3"]))
	}
}

func TestClose(_ *testing.T) {
	repo := NewRepository()
	repo.Close()
}
