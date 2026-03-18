package lsh

import (
	"sync"
	"testing"
)

func TestComputeBands_TableDriven(t *testing.T) {
	bands := 3
	rows := 2

	testcases := []struct {
		name      string
		bands     int
		rows      int
		signature []uint64
		check     func(t *testing.T, res []string, err error)
	}{
		{
			name:      "standard signature",
			bands:     3,
			rows:      2,
			signature: []uint64{1, 1, 2, 2, 3, 3},
			check: func(t *testing.T, res []string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if len(res) != 3 {
					t.Errorf("expected 3 band keys, got %d", len(res))
				}

				if res[0] != "b0:ed7aead4b89ab6fc" {
					t.Errorf("band 0 malformed: %s", res[0])
				}

				if res[2] != "b2:75ec20e3cf098126" {
					t.Errorf("band 2 malformed: %s", res[2])
				}
			},
		},
		{
			name:      "all zeros",
			bands:     3,
			rows:      2,
			signature: []uint64{0, 0, 0, 0, 0, 0},
			check: func(t *testing.T, res []string, err error) {
				if err != nil {
					t.Fatal(err)
				}

				if len(res) != 3 {
					t.Error("zeros signature produced wrong number of bands")
				}

				if res[0] == res[1] {
					t.Error("bands collided: prefixes (b0 vs b1) failed to separate identical data")
				}
			},
		},
		{
			name:      "short signature",
			bands:     3,
			rows:      2,
			signature: []uint64{1, 2, 3},
			check: func(t *testing.T, _ []string, err error) {
				if err != ErrSignatureTooShort {
					t.Errorf("expected ErrSignatureTooShort, got %v", err)
				}
			},
		},
		{
			name:      "empty signature",
			bands:     3,
			rows:      2,
			signature: []uint64{},
			check: func(t *testing.T, _ []string, err error) {
				if err != ErrSignatureTooShort {
					t.Errorf("expected ErrSignatureTooShort, got %v", err)
				}
			},
		},
		{
			name:      "nil signature",
			bands:     3,
			rows:      2,
			signature: nil,
			check: func(t *testing.T, _ []string, err error) {
				if err != ErrSignatureTooShort {
					t.Errorf("expected ErrSignatureTooShort, got %v", err)
				}
			},
		},
		{
			name:      "extra elements beyond bands*rows are ignored",
			bands:     2,
			rows:      2,
			signature: []uint64{1, 2, 3, 4, 99, 100},
			check: func(t *testing.T, res []string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if len(res) != 2 {
					t.Errorf("expected 2 band keys, got %d", len(res))
				}
			},
		},
		{
			name:      "single band single row",
			bands:     1,
			rows:      1,
			signature: []uint64{42},
			check: func(t *testing.T, res []string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if len(res) != 1 {
					t.Errorf("expected 1 band key, got %d", len(res))
				}
			},
		},
		{
			name:      "max uint64 values",
			bands:     2,
			rows:      2,
			signature: []uint64{^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0)},
			check: func(t *testing.T, res []string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if len(res) != 2 {
					t.Errorf("expected 2 band keys, got %d", len(res))
				}
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			b, r := tc.bands, tc.rows
			if b == 0 && r == 0 {
				b = bands
				r = rows
			}

			res, err := ComputeBands(tc.signature, b, r)
			tc.check(t, res, err)
		})
	}

	// Behavioral: deterministic output for same input
	t.Run("deterministic", func(t *testing.T) {
		sig := []uint64{10, 20, 30, 40, 50, 60}

		res1, _ := ComputeBands(sig, bands, rows)
		res2, _ := ComputeBands(sig, bands, rows)

		for i := range res1 {
			if res1[i] != res2[i] {
				t.Errorf("non-deterministic at band %d: %s vs %s", i, res1[i], res2[i])
			}
		}
	})

	// Behavioral: different signatures produce different keys
	t.Run("different sigs produce different keys", func(t *testing.T) {
		sig1 := []uint64{1, 2, 3, 4, 5, 6}
		sig2 := []uint64{7, 8, 9, 10, 11, 12}

		res1, _ := ComputeBands(sig1, bands, rows)
		res2, _ := ComputeBands(sig2, bands, rows)

		allSame := true

		for i := range res1 {
			if res1[i] != res2[i] {
				allSame = false

				break
			}
		}

		if allSame {
			t.Error("different signatures produced identical band keys")
		}
	})
}

func TestPrefixKeys(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		keys   []string
		want   []string
	}{
		{name: "standard", prefix: "v1", keys: []string{"b0:abc", "b1:def"}, want: []string{"v1:b0:abc", "v1:b1:def"}},
		{name: "empty prefix", prefix: "", keys: []string{"b0:abc"}, want: []string{":b0:abc"}},
		{name: "empty keys", prefix: "v1", keys: []string{}, want: []string{}},
		{name: "nil keys", prefix: "v1", keys: nil, want: []string{}},
		{name: "single key", prefix: "pfx", keys: []string{"k"}, want: []string{"pfx:k"}},
		{name: "long prefix", prefix: "abc123def456", keys: []string{"x"}, want: []string{"abc123def456:x"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := PrefixKeys(tc.prefix, tc.keys)

			if len(result) != len(tc.want) {
				t.Fatalf("len: got %d, want %d", len(result), len(tc.want))
			}

			for i := range result {
				if result[i] != tc.want[i] {
					t.Errorf("[%d]: got %s, want %s", i, result[i], tc.want[i])
				}
			}
		})
	}
}

func TestSignatureSize(t *testing.T) {
	cases := []struct {
		name  string
		bands int
		rows  int
		want  int
	}{
		{name: "standard", bands: 20, rows: 5, want: 100},
		{name: "single", bands: 1, rows: 1, want: 1},
		{name: "zero bands", bands: 0, rows: 5, want: 0},
		{name: "zero rows", bands: 5, rows: 0, want: 0},
		{name: "both zero", bands: 0, rows: 0, want: 0},
		{name: "large config", bands: 100, rows: 10, want: 1000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Bands: tc.bands, Rows: tc.rows}
			if got := cfg.SignatureSize(); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNewSignaturePool(t *testing.T) {
	pool := NewSignaturePool(10)

	sigPtr := pool.Get().(*[]uint64)
	sig := *sigPtr

	if len(sig) != 10 {
		t.Fatalf("expected len 10, got %d", len(sig))
	}

	pool.Put(sigPtr)

	sigPtr2 := pool.Get().(*[]uint64)
	if len(*sigPtr2) != 10 {
		t.Fatalf("expected len 10 after reuse, got %d", len(*sigPtr2))
	}
}

func TestGroupShard(t *testing.T) {
	cases := []struct {
		name  string
		group string
	}{
		{name: "simple", group: "email"},
		{name: "empty", group: ""},
		{name: "long", group: "a_very_long_group_name_that_should_still_work"},
		{name: "unicode", group: "группа"},
		{name: "special chars", group: "org/sub-group:123"},
		{name: "single char", group: "x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shard := GroupShard(tc.group)
			if shard >= GroupLockShards {
				t.Errorf("shard %d >= max %d", shard, GroupLockShards)
			}
		})
	}

	// Behavioral: deterministic
	t.Run("deterministic", func(t *testing.T) {
		s1 := GroupShard("test")
		s2 := GroupShard("test")

		if s1 != s2 {
			t.Errorf("non-deterministic: %d vs %d", s1, s2)
		}
	})

	// Behavioral: different groups can map to different shards
	t.Run("different groups diverge", func(t *testing.T) {
		s1 := GroupShard("alpha")
		s2 := GroupShard("beta")

		if s1 == s2 {
			t.Log("warning: different groups mapped to same shard (possible but unlikely)")
		}
	})
}

func TestGroupShard_Distribution(t *testing.T) {
	seen := make(map[uint32]bool)

	for i := 0; i < 1000; i++ {
		s := GroupShard("group_" + string(rune(i)))
		seen[s] = true
	}

	if len(seen) < 10 {
		t.Errorf("poor distribution: only %d unique shards from 1000 inputs", len(seen))
	}
}

func TestHashVersion_EncodeError(t *testing.T) {
	// packer.Encode returns error for unsupported types (e.g. struct{})
	_, err := HashVersion("grp", struct{}{})
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestHashVersion(t *testing.T) {
	h1, err := HashVersion("grp", 20, 5, 3, 0.6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h2, err := HashVersion("grp", 20, 5, 3, 0.6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 != h2 {
		t.Errorf("non-deterministic: %s vs %s", h1, h2)
	}

	h3, err := HashVersion("grp", 21, 5, 3, 0.6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 == h3 {
		t.Error("different fields produced same hash")
	}

	h4, err := HashVersion("other", 20, 5, 3, 0.6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 == h4 {
		t.Error("different group produced same hash")
	}
}

func TestPrefixKeys_Empty(t *testing.T) {
	result := PrefixKeys("pfx", nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestSignaturePool_Concurrent(t *testing.T) {
	pool := NewSignaturePool(50)

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			p := pool.Get().(*[]uint64)
			s := *p

			if len(s) != 50 {
				t.Errorf("expected len 50, got %d", len(s))
			}

			pool.Put(p)
		}()
	}

	wg.Wait()
}
