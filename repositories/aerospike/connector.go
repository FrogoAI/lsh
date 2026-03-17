package aerospike

import (
	as "github.com/aerospike/aerospike-client-go/v8"
	"github.com/aerospike/aerospike-client-go/v8/types"

	"github.com/FrogoAI/lsh/repositories"
)

const (
	binMembers = "m" // Map<MemberID, Metadata>
	binKV      = "kv"

	defaultHardLimit = 10000

	bucketTTL = 14 * 24 * 3600 // 14 days
	recordTTL = 90 * 24 * 3600 // 90 days
)

type Repository struct {
	client    *as.Client
	namespace string
	set       string

	writePolicy *as.WritePolicy
	readPolicy  *as.BasePolicy

	hardLimit int
}

func NewRepository(client *as.Client, namespace, set string, hardLimit int) *Repository {
	if hardLimit <= 0 {
		hardLimit = defaultHardLimit
	}

	return &Repository{
		client:      client,
		namespace:   namespace,
		set:         set,
		writePolicy: as.NewWritePolicy(0, 0),
		readPolicy:  as.NewPolicy(),
		hardLimit:   hardLimit,
	}
}

func (r *Repository) AddBucketMember(bucketKey, memberID string, metadata int64) error {
	key, err := as.NewKey(r.namespace, r.set, bucketKey)
	if err != nil {
		return err
	}

	mapPolicy := as.NewMapPolicy(as.MapOrder.UNORDERED, as.MapWriteMode.UPDATE)
	op := as.MapPutOp(mapPolicy, binMembers, memberID, metadata)

	wp := *r.writePolicy
	wp.Expiration = bucketTTL

	_, err = r.client.Operate(&wp, key, op)

	return err
}

func (r *Repository) BatchAddBucketMember(bucketKeys []string, memberID string, metadata int64) error {
	if len(bucketKeys) == 0 {
		return nil
	}

	mapPolicy := as.NewMapPolicy(as.MapOrder.UNORDERED, as.MapWriteMode.UPDATE)
	op := as.MapPutOp(mapPolicy, binMembers, memberID, metadata)

	records := make([]as.BatchRecordIfc, len(bucketKeys))

	wp := as.NewBatchWritePolicy()
	wp.Expiration = bucketTTL

	for i, bKey := range bucketKeys {
		key, err := as.NewKey(r.namespace, r.set, bKey)
		if err != nil {
			return err
		}

		records[i] = as.NewBatchWrite(wp, key, op)
	}

	return r.client.BatchOperate(nil, records)
}

func (r *Repository) GetBucketMembers(bucketKey string) ([]repositories.BucketMember, error) {
	key, err := as.NewKey(r.namespace, r.set, bucketKey)
	if err != nil {
		return nil, err
	}

	opSize := as.MapSizeOp(binMembers)

	record, err := r.client.Operate(r.writePolicy, key, opSize)
	if err != nil {
		if err.Matches(types.KEY_NOT_FOUND_ERROR) {
			return nil, nil
		}

		return nil, err
	}

	sizeInt := 0
	if res, ok := record.Bins[binMembers].(int); ok {
		sizeInt = res
	} else {
		return nil, nil
	}

	if sizeInt > r.hardLimit {
		stub := make([]repositories.BucketMember, sizeInt)

		return stub, nil
	}

	fullRecord, err := r.client.Get(r.readPolicy, key, binMembers)
	if err != nil {
		return nil, err
	}

	rawMap, ok := fullRecord.Bins[binMembers].(map[interface{}]interface{})
	if !ok {
		return nil, nil
	}

	members := make([]repositories.BucketMember, 0, len(rawMap))

	for k, v := range rawMap {
		idStr, ok := k.(string)
		if !ok {
			continue
		}

		var meta int64

		switch n := v.(type) {
		case int:
			meta = int64(n)
		case int64:
			meta = n
		case float64:
			meta = int64(n)
		}

		members = append(members, repositories.BucketMember{ID: idStr, Metadata: meta})
	}

	return members, nil
}

func (r *Repository) BatchGetBucketMembers(bucketKeys []string) (map[string][]repositories.BucketMember, error) {
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

	result := make(map[string][]repositories.BucketMember, len(bucketKeys))

	for i, rec := range records {
		if rec == nil {
			continue
		}

		rawMap, ok := rec.Bins[binMembers].(map[interface{}]interface{})
		if !ok {
			continue
		}

		bKey := bucketKeys[i]

		for id, v := range rawMap {
			idStr, ok := id.(string)
			if !ok {
				continue
			}

			var meta int64

			switch n := v.(type) {
			case int:
				meta = int64(n)
			case int64:
				meta = n
			case float64:
				meta = int64(n)
			}

			result[bKey] = append(result[bKey], repositories.BucketMember{ID: idStr, Metadata: meta})
		}
	}

	return result, nil
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
