package lsh

import (
	"context"
	"strconv"
	"testing"

	pp "github.com/k0kubun/pp/v3"

	"github.com/FrogoAI/lsh/repositories/memory"
	"github.com/FrogoAI/testutils"
)

func TestUpsert(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()

	cfg, err := GetLSHConfigFromEnv()
	testutils.Equal(t, err, nil)

	service := NewSimilarityService(repo, cfg)

	id1, err := service.Upsert(ctx, "users", "John Doe")
	testutils.Equal(t, err, nil)

	if id1 == "" {
		t.Fatalf("We must have unempty bucket, got: %s", id1)
	}

	id2, err := service.Upsert(ctx, "users", "Johnn Doe")
	testutils.Equal(t, err, nil)

	if id2 == "" {
		t.Fatalf("We must have unempty bucket, got: %s", id2)
	}

	testutils.Equal(t, id1, id2)

	id3, err := service.Upsert(ctx, "users", "Jane Doe")
	testutils.Equal(t, err, nil)

	if id3 == "" {
		t.Fatalf("We must have unempty bucket, got: %s", id2)
	}

	for i := 0; i < 100; i++ {
		_, err := service.Upsert(ctx, "users", "test"+strconv.Itoa(i))
		testutils.Equal(t, err, nil)
	}

	pp.Print(repo)

	testutils.NotEqual(t, id1, id3)
}
