package vector

import (
	"testing"

	"github.com/FrogoAI/lsh/v2/repositories"
)

func TestRecordFromBins(t *testing.T) {
	cases := []struct {
		name   string
		bins   map[string]any
		wantOK bool
	}{
		{
			name:   "valid with native types",
			bins:   map[string]any{"v": []float64{1.0, 2.0}, "g": "grp", "s": []uint64{0, 1}},
			wantOK: true,
		},
		{
			name: "valid with []any vector (mixed numeric)",
			bins: map[string]any{
				"v": []any{float64(1.0), int(2), int64(3)},
				"g": "grp",
				"s": []any{int(0), int64(1), float64(0)},
			},
			wantOK: true,
		},
		{
			name:   "missing vector returns false",
			bins:   map[string]any{"g": "grp", "s": []uint64{1}},
			wantOK: false,
		},
		{
			name:   "empty bins",
			bins:   map[string]any{},
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := recordFromBins(repositories.Record{Key: "k", Bins: tc.bins})
			if ok != tc.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tc.wantOK)
			}

			if tc.wantOK {
				if rec.ID != "k" {
					t.Errorf("ID: got %s, want k", rec.ID)
				}

				if len(rec.Vector) == 0 {
					t.Error("expected non-empty vector")
				}
			}
		})
	}
}

func TestRecordToBins(t *testing.T) {
	rec := Record{
		ID:        "u1",
		Vector:    []float64{1.0, 2.0},
		GroupID:   "grp",
		Signature: []uint64{0, 1},
	}

	bins := rec.toBins()

	if bins["v"] == nil {
		t.Error("expected vector bin")
	}

	if bins["g"] != "grp" {
		t.Errorf("expected group grp, got %v", bins["g"])
	}
}
