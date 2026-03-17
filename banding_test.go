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
		signature []uint64
		check     func(t *testing.T, res []string, err error)
	}{
		{
			name:      "standard signature",
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
			signature: []uint64{1, 2, 3},
			check: func(t *testing.T, _ []string, err error) {
				if err != ErrSignatureTooShort {
					t.Errorf("expected ErrSignatureTooShort, got %v", err)
				}
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ComputeBands(tc.signature, bands, rows)
			tc.check(t, res, err)
		})
	}
}

func TestPrefixKeys(t *testing.T) {
	keys := []string{"b0:abc", "b1:def"}
	result := PrefixKeys("v1", keys)

	if len(result) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(result))
	}

	if result[0] != "v1:b0:abc" {
		t.Errorf("expected v1:b0:abc, got %s", result[0])
	}

	if result[1] != "v1:b1:def" {
		t.Errorf("expected v1:b1:def, got %s", result[1])
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
		{name: "zero", bands: 0, rows: 5, want: 0},
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shard := GroupShard(tc.group)
			if shard >= GroupLockShards {
				t.Errorf("shard %d >= max %d", shard, GroupLockShards)
			}
		})
	}

	// deterministic
	s1 := GroupShard("test")
	s2 := GroupShard("test")

	if s1 != s2 {
		t.Errorf("non-deterministic: %d vs %d", s1, s2)
	}
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
