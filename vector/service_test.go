package vector

import (
	"context"
	"testing"

	"github.com/FrogoAI/lsh"
	"github.com/FrogoAI/lsh/repositories/memory"
)

func vectorConfig(dims int) *Config {
	return &Config{
		Config: lsh.Config{
			Bands: 20, Rows: 5,
			MaxBucketSize: 200, MaxTotalCandidates: 100, Seed: 42,
		},
		VectorDimensions: dims,
		CosineThreshold:  0.7,
	}
}

func TestIndex_Basic(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := NewService(repo, vectorConfig(3))

	err := svc.Index(ctx, "grp", "user1", []float64{1.0, 2.0, 3.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs, err := repo.GetRecords([]string{"user1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
}

func TestIndex_Validation(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := NewService(repo, vectorConfig(3))

	cases := []struct {
		name    string
		vec     []float64
		wantErr error
	}{
		{name: "empty vector", vec: []float64{}, wantErr: ErrEmptyVector},
		{name: "nil vector", vec: nil, wantErr: ErrEmptyVector},
		{name: "wrong dimension", vec: []float64{1.0, 2.0}, wantErr: ErrWrongDimension},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.Index(ctx, "grp", "u", tc.vec)
			if err != tc.wantErr {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestFindSimilar_CloseVectors(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(4)
	cfg.CosineThreshold = 0.5
	svc := NewService(repo, cfg)

	v1 := []float64{1.0, 2.0, 3.0, 4.0}
	v2 := []float64{1.1, 2.1, 3.1, 4.1}

	err := svc.Index(ctx, "grp", "user1", v1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matches, err := svc.FindSimilar(ctx, "grp", v2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) == 0 {
		t.Fatal("expected at least one match for close vectors")
	}

	if matches[0].UserID != "user1" {
		t.Errorf("expected user1, got %s", matches[0].UserID)
	}

	if matches[0].Similarity < 0.99 {
		t.Errorf("expected high similarity, got %f", matches[0].Similarity)
	}
}

func TestFindSimilar_FarVectors(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(4)
	cfg.CosineThreshold = 0.9
	svc := NewService(repo, cfg)

	err := svc.Index(ctx, "grp", "user1", []float64{1.0, 0.0, 0.0, 0.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matches, err := svc.FindSimilar(ctx, "grp", []float64{0.0, 0.0, 0.0, 1.0}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) != 0 {
		t.Errorf("expected no matches for orthogonal vectors, got %d", len(matches))
	}
}

func TestFindSimilar_TopK(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(3)
	cfg.CosineThreshold = 0.3
	cfg.Bands = 40
	svc := NewService(repo, cfg)

	base := []float64{1.0, 1.0, 1.0}

	for i := 0; i < 10; i++ {
		v := []float64{1.0 + float64(i)*0.01, 1.0, 1.0}

		err := svc.Index(ctx, "grp", "user"+string(rune('a'+i)), v)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	matches, err := svc.FindSimilar(ctx, "grp", base, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) > 3 {
		t.Errorf("expected at most 3 matches, got %d", len(matches))
	}
}

func TestGetBucketPeers(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(3)
	svc := NewService(repo, cfg)

	err := svc.Index(ctx, "grp", "user1", []float64{1.0, 2.0, 3.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	peers, err := svc.GetBucketPeers(ctx, "grp", []float64{1.0, 2.0, 3.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(peers) == 0 {
		t.Error("expected at least one peer")
	}
}

func TestFindSimilar_Validation(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := NewService(repo, vectorConfig(3))

	cases := []struct {
		name    string
		vec     []float64
		wantErr error
	}{
		{name: "empty vector", vec: []float64{}, wantErr: ErrEmptyVector},
		{name: "nil vector", vec: nil, wantErr: ErrEmptyVector},
		{name: "wrong dimension", vec: []float64{1.0, 2.0}, wantErr: ErrWrongDimension},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.FindSimilar(ctx, "grp", tc.vec, 10)
			if err != tc.wantErr {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestGetBucketPeers_Validation(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := NewService(repo, vectorConfig(3))

	cases := []struct {
		name    string
		vec     []float64
		wantErr error
	}{
		{name: "empty vector", vec: []float64{}, wantErr: ErrEmptyVector},
		{name: "nil vector", vec: nil, wantErr: ErrEmptyVector},
		{name: "wrong dimension", vec: []float64{1.0, 2.0}, wantErr: ErrWrongDimension},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.GetBucketPeers(ctx, "grp", tc.vec)
			if err != tc.wantErr {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestFindSimilar_NoCandidates(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := NewService(repo, vectorConfig(3))

	matches, err := svc.FindSimilar(ctx, "grp", []float64{1.0, 2.0, 3.0}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if matches != nil {
		t.Errorf("expected nil matches for empty index, got %v", matches)
	}
}

func TestGetBucketPeers_MultiplePeers(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(3)
	cfg.Bands = 40
	svc := NewService(repo, cfg)

	_ = svc.Index(ctx, "grp", "user1", []float64{1.0, 2.0, 3.0})
	_ = svc.Index(ctx, "grp", "user2", []float64{1.1, 2.1, 3.1})
	_ = svc.Index(ctx, "grp", "user3", []float64{-10.0, -20.0, -30.0})

	peers, err := svc.GetBucketPeers(ctx, "grp", []float64{1.0, 2.0, 3.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(peers) < 1 {
		t.Error("expected at least one peer")
	}
}

func TestFindSimilar_MaxBucketSizeSkip(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(3)
	cfg.MaxBucketSize = 1
	cfg.Bands = 5
	cfg.Rows = 2
	svc := NewService(repo, cfg)

	for i := 0; i < 5; i++ {
		_ = svc.Index(ctx, "grp", "user"+string(rune('a'+i)), []float64{1.0, 2.0, 3.0})
	}

	// Should not error even though buckets are oversized
	_, err := svc.FindSimilar(ctx, "grp", []float64{1.0, 2.0, 3.0}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func BenchmarkIndex(b *testing.B) {
	ctx := context.Background()
	repo := memory.NewRepository()
	cfg := vectorConfig(18)
	svc := NewService(repo, cfg)

	vec := make([]float64, 18)
	for i := range vec {
		vec[i] = float64(i) * 0.1
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vec[0] = float64(i)
		_ = svc.Index(ctx, "grp", "u", vec)
	}
}
