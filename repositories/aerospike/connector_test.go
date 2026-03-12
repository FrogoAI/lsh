//go:build integration

package aerospike

import (
	"testing"

	as "github.com/aerospike/aerospike-client-go/v8"

	"github.com/FrogoAI/lsh/model"
	"github.com/FrogoAI/testutils"
)

const (
	testHost      = "127.0.0.1"
	testPort      = 3000
	testNamespace = "test"
	testSet       = "lsh_connector_test"
)

func newTestRepo(t *testing.T) *Repository {
	t.Helper()

	client, err := as.NewClient(testHost, testPort)
	if err != nil {
		t.Fatalf("failed to connect to aerospike: %v", err)
	}

	t.Cleanup(func() { client.Close() })

	return NewRepository(client, testNamespace, testSet, 5000)
}

func TestNewRepository(t *testing.T) {
	cases := []struct {
		name          string
		hardLimit     int
		wantHardLimit int
	}{
		{name: "custom limit", hardLimit: 3000, wantHardLimit: 3000},
		{name: "zero uses default", hardLimit: 0, wantHardLimit: defaultHardLimit},
		{name: "negative uses default", hardLimit: -1, wantHardLimit: defaultHardLimit},
	}

	client, err := as.NewClient(testHost, testPort)
	if err != nil {
		t.Fatalf("failed to connect to aerospike: %v", err)
	}

	defer client.Close()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewRepository(client, testNamespace, testSet, tc.hardLimit)
			testutils.Equal(t, tc.wantHardLimit, repo.hardLimit)
		})
	}
}

func TestAddToBucket_And_GetBucketMembers(t *testing.T) {
	repo := newTestRepo(t)

	cases := []struct {
		name      string
		bucketKey string
		value     string
		length    int
	}{
		{name: "first member", bucketKey: "test_bucket_1", value: "user1", length: 5},
		{name: "second member", bucketKey: "test_bucket_1", value: "user2", length: 8},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := repo.AddToBucket(tc.bucketKey, tc.value, tc.length)
			testutils.Equal(t, err, nil)
		})
	}

	ids, lens, err := repo.GetBucketMembers("test_bucket_1")
	testutils.Equal(t, err, nil)
	testutils.Equal(t, 2, len(ids))
	testutils.Equal(t, 2, len(lens))
}

func TestGetBucketMembers_NotFound(t *testing.T) {
	repo := newTestRepo(t)

	ids, lens, err := repo.GetBucketMembers("nonexistent_bucket_xyz")
	testutils.Equal(t, err, nil)
	testutils.Equal(t, 0, len(ids))
	testutils.Equal(t, 0, len(lens))
}

func TestSaveRecord_And_GetRecords(t *testing.T) {
	repo := newTestRepo(t)

	cases := []struct {
		name   string
		record model.Record
	}{
		{
			name: "basic record",
			record: model.Record{
				ID: "rec_test_1", Input: "hello@example.com",
				GroupID: "email", Signature: []uint64{1, 2, 3},
			},
		},
		{
			name: "another record",
			record: model.Record{
				ID: "rec_test_2", Input: "world@example.com",
				GroupID: "email", Signature: []uint64{4, 5, 6},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := repo.SaveRecord(tc.record)
			testutils.Equal(t, err, nil)
		})
	}

	// Fetch both
	results, err := repo.GetRecords([]string{"rec_test_1", "rec_test_2"})
	testutils.Equal(t, err, nil)
	testutils.Equal(t, 2, len(results))
	testutils.Equal(t, "hello@example.com", results["rec_test_1"].Input)
	testutils.Equal(t, "world@example.com", results["rec_test_2"].Input)
}

func TestGetRecords_Empty(t *testing.T) {
	repo := newTestRepo(t)

	results, err := repo.GetRecords([]string{})
	testutils.Equal(t, err, nil)
	testutils.Equal(t, 0, len(results))
}

func TestGetRecords_NotFound(t *testing.T) {
	repo := newTestRepo(t)

	results, err := repo.GetRecords([]string{"nonexistent_id_xyz"})
	testutils.Equal(t, err, nil)
	testutils.Equal(t, 0, len(results))
}

func TestBatchAddToBuckets_And_BatchGetBuckets(t *testing.T) {
	repo := newTestRepo(t)

	keys := []string{"batch_b1", "batch_b2", "batch_b3"}

	err := repo.BatchAddToBuckets(keys, "user_batch", 10)
	testutils.Equal(t, err, nil)

	members, lens, err := repo.BatchGetBuckets(keys)
	testutils.Equal(t, err, nil)

	for _, k := range keys {
		if len(members[k]) == 0 {
			t.Errorf("expected members for key %q", k)
		}

		if len(lens[k]) == 0 {
			t.Errorf("expected lens for key %q", k)
		}
	}
}

func TestBatchAddToBuckets_Empty(t *testing.T) {
	repo := newTestRepo(t)

	err := repo.BatchAddToBuckets([]string{}, "val", 1)
	testutils.Equal(t, err, nil)
}

func TestSaveResolvedID_And_GetResolvedID(t *testing.T) {
	repo := newTestRepo(t)

	cases := []struct {
		name       string
		bid        string
		resolvedID string
	}{
		{name: "basic mapping", bid: "res_bid_1", resolvedID: "res_resolved_1"},
		{name: "overwrite mapping", bid: "res_bid_1", resolvedID: "res_resolved_updated"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := repo.SaveResolvedID(tc.bid, tc.resolvedID)
			testutils.Equal(t, err, nil)

			got, err := repo.GetResolvedID(tc.bid)
			testutils.Equal(t, err, nil)
			testutils.Equal(t, tc.resolvedID, got)
		})
	}
}

func TestGetResolvedID_NotFound(t *testing.T) {
	repo := newTestRepo(t)

	got, err := repo.GetResolvedID("nonexistent_res_xyz")
	testutils.Equal(t, err, nil)
	testutils.Equal(t, "", got)
}

func TestClose(t *testing.T) {
	client, err := as.NewClient(testHost, testPort)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	repo := NewRepository(client, testNamespace, testSet, 0)
	repo.Close()

	// Client should be closed — verify by checking IsConnected
	testutils.Equal(t, false, client.IsConnected())
}
