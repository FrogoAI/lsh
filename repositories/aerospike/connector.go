package aerospike

import (
	"log/slog"

	as "github.com/aerospike/aerospike-client-go/v8"
	"github.com/aerospike/aerospike-client-go/v8/types"

	"github.com/FrogoAI/lsh/v2/repositories"
)

const (
	binMembers = "m" // Map<MemberID, Metadata>
	binKV      = "kv"

	bucketTTL = 14 * 24 * 3600 // 14 days
	recordTTL = 90 * 24 * 3600 // 90 days
)

type Repository struct {
	client    *as.Client
	namespace string
	set       string

	writePolicy *as.WritePolicy
	readPolicy  *as.BasePolicy

	maxBucketReps int
}

func NewRepository(client *as.Client, namespace, set string, opts ...Option) *Repository {
	r := &Repository{
		client:      client,
		namespace:   namespace,
		set:         set,
		writePolicy: as.NewWritePolicy(0, 0),
		readPolicy:  as.NewPolicy(),
	}

	for _, o := range opts {
		o(r)
	}

	return r
}

// Option configures the Aerospike repository.
type Option func(*Repository)

// WithMaxBucketReps sets the maximum number of representatives per bucket.
// When exceeded, the oldest entries (by key order) are trimmed.
// 0 (default) means unlimited.
func WithMaxBucketReps(n int) Option {
	return func(r *Repository) {
		r.maxBucketReps = n
	}
}

func (r *Repository) SetRepresentative(bucketKey, memberID string, metadata int64) error {
	key, err := as.NewKey(r.namespace, r.set, bucketKey)
	if err != nil {
		return err
	}

	mapPolicy := as.NewMapPolicy(as.MapOrder.KEY_ORDERED, as.MapWriteMode.UPDATE)

	wp := *r.writePolicy
	wp.Expiration = bucketTTL

	ops := []*as.Operation{
		as.MapPutOp(mapPolicy, binMembers, memberID, metadata),
		as.MapSizeOp(binMembers),
	}

	record, err := r.client.Operate(&wp, key, ops...)
	if err != nil {
		return err
	}

	r.trimIfNeeded(&wp, key, record)

	return nil
}

func (r *Repository) BatchSetRepresentative(bucketKeys []string, memberID string, metadata int64) error {
	if len(bucketKeys) == 0 {
		return nil
	}

	mapPolicy := as.NewMapPolicy(as.MapOrder.KEY_ORDERED, as.MapWriteMode.UPDATE)
	putOp := as.MapPutOp(mapPolicy, binMembers, memberID, metadata)
	sizeOp := as.MapSizeOp(binMembers)

	records := make([]as.BatchRecordIfc, len(bucketKeys))

	wp := as.NewBatchWritePolicy()
	wp.Expiration = bucketTTL

	for i, bKey := range bucketKeys {
		key, err := as.NewKey(r.namespace, r.set, bKey)
		if err != nil {
			return err
		}

		records[i] = as.NewBatchWrite(wp, key, putOp, sizeOp)
	}

	if err := r.client.BatchOperate(nil, records); err != nil {
		return err
	}

	if r.maxBucketReps <= 0 {
		return nil
	}

	// Trim oversized buckets (best-effort, fire-and-forget per key)
	trimWP := *r.writePolicy
	trimWP.Expiration = bucketTTL

	for i, br := range records {
		bw, ok := br.(*as.BatchWrite)
		if !ok || bw.Record == nil {
			continue
		}

		size := extractMapSize(bw.Record.Bins[binMembers])
		if size <= r.maxBucketReps {
			continue
		}

		key, err := as.NewKey(r.namespace, r.set, bucketKeys[i])
		if err != nil {
			continue
		}

		excess := size - r.maxBucketReps
		trimOp := as.MapRemoveByIndexRangeCountOp(binMembers, 0, excess, as.MapReturnType.NONE)

		if _, err := r.client.Operate(&trimWP, key, trimOp); err != nil {
			slog.Warn("failed to trim bucket",
				slog.String("bucket", bucketKeys[i]),
				slog.Int("size", size),
				slog.Int("max", r.maxBucketReps),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

// trimIfNeeded removes excess entries from a bucket after a MapPut+MapSize Operate.
func (r *Repository) trimIfNeeded(wp *as.WritePolicy, key *as.Key, record *as.Record) {
	if r.maxBucketReps <= 0 || record == nil {
		return
	}

	size := extractMapSize(record.Bins[binMembers])
	if size <= r.maxBucketReps {
		return
	}

	excess := size - r.maxBucketReps
	trimOp := as.MapRemoveByIndexRangeCountOp(binMembers, 0, excess, as.MapReturnType.NONE)

	if _, err := r.client.Operate(wp, key, trimOp); err != nil {
		slog.Warn("failed to trim bucket",
			slog.String("key", key.String()),
			slog.Int("size", size),
			slog.Int("max", r.maxBucketReps),
			slog.Any("error", err),
		)
	}
}

func (r *Repository) GetRepresentatives(bucketKey string) ([]repositories.Representative, error) {
	key, err := as.NewKey(r.namespace, r.set, bucketKey)
	if err != nil {
		return nil, err
	}

	record, err := r.client.Get(r.readPolicy, key, binMembers)
	if err != nil {
		if err.Matches(types.KEY_NOT_FOUND_ERROR) {
			return nil, nil
		}

		return nil, err
	}

	return parseBinRepresentatives(record.Bins[binMembers]), nil
}

func (r *Repository) BatchGetRepresentatives(bucketKeys []string) (map[string][]repositories.Representative, error) {
	keys := make([]*as.Key, len(bucketKeys))
	for i, k := range bucketKeys {
		key, err := as.NewKey(r.namespace, r.set, k)
		if err != nil {
			return nil, err
		}

		keys[i] = key
	}

	records, err := r.client.BatchGet(nil, keys, binMembers)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]repositories.Representative, len(bucketKeys))

	for i, rec := range records {
		if rec == nil {
			continue
		}

		reps := parseBinRepresentatives(rec.Bins[binMembers])
		if len(reps) == 0 {
			continue
		}

		result[bucketKeys[i]] = reps
	}

	return result, nil
}

// parseBinRepresentatives handles both Aerospike map formats:
// - map[interface{}]interface{} (UNORDERED maps)
// - []as.MapPair (KEY_ORDERED maps)
func parseBinRepresentatives(bin interface{}) []repositories.Representative {
	switch v := bin.(type) {
	case map[interface{}]interface{}:
		return parseMapRepresentatives(v)
	case []as.MapPair:
		return parsePairRepresentatives(v)
	default:
		return nil
	}
}

func parseMapRepresentatives(rawMap map[interface{}]interface{}) []repositories.Representative {
	reps := make([]repositories.Representative, 0, len(rawMap))

	for k, v := range rawMap {
		idStr, ok := k.(string)
		if !ok {
			continue
		}

		reps = append(reps, repositories.Representative{ID: idStr, Metadata: toInt64(v)})
	}

	return reps
}

func parsePairRepresentatives(pairs []as.MapPair) []repositories.Representative {
	reps := make([]repositories.Representative, 0, len(pairs))

	for _, p := range pairs {
		idStr, ok := p.Key.(string)
		if !ok {
			continue
		}

		reps = append(reps, repositories.Representative{ID: idStr, Metadata: toInt64(p.Value)})
	}

	return reps
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

// extractMapSize extracts the map size from an Operate result.
// The result of MapPut+MapSize via Operate is OpResults ([]interface{}),
// where the second element is the map size.
func extractMapSize(bin interface{}) int {
	switch v := bin.(type) {
	case int:
		return v
	case as.OpResults:
		if len(v) >= 2 { //nolint:mnd
			if size, ok := v[1].(int); ok {
				return size
			}
		}
	}

	return 0
}

func (r *Repository) SaveRecord(key string, bins map[string]any) error {
	asKey, err := as.NewKey(r.namespace, r.set, key)
	if err != nil {
		return err
	}

	asBins := as.BinMap(bins)

	wp := *r.writePolicy
	wp.Expiration = recordTTL

	return r.client.Put(&wp, asKey, asBins)
}

func (r *Repository) GetRecords(keys []string) ([]repositories.Record, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	asKeys := make([]*as.Key, len(keys))
	for i, id := range keys {
		k, err := as.NewKey(r.namespace, r.set, id)
		if err != nil {
			return nil, err
		}

		asKeys[i] = k
	}

	records, err := r.client.BatchGet(as.NewBatchPolicy(), asKeys)
	if err != nil {
		return nil, err
	}

	var result []repositories.Record

	for i, rec := range records {
		if rec == nil {
			continue
		}

		bins := make(map[string]any, len(rec.Bins))
		for k, v := range rec.Bins {
			bins[k] = v
		}

		result = append(result, repositories.Record{Key: keys[i], Bins: bins})
	}

	return result, nil
}

func (r *Repository) PutValue(key, value string) error {
	asKey, err := as.NewKey(r.namespace, r.set, "kv:"+key)
	if err != nil {
		return err
	}

	wp := *r.writePolicy
	wp.Expiration = recordTTL

	return r.client.Put(&wp, asKey, as.BinMap{binKV: value})
}

func (r *Repository) GetValue(key string) (string, error) {
	asKey, err := as.NewKey(r.namespace, r.set, "kv:"+key)
	if err != nil {
		return "", err
	}

	rec, err := r.client.Get(r.readPolicy, asKey, binKV)
	if err != nil {
		if err.Matches(types.KEY_NOT_FOUND_ERROR) {
			return "", nil
		}

		return "", err
	}

	if v, ok := rec.Bins[binKV].(string); ok {
		return v, nil
	}

	return "", nil
}

func (r *Repository) Close() {
	if r.client != nil {
		r.client.Close()
	}
}
