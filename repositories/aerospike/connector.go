package aerospike

import (
	as "github.com/aerospike/aerospike-client-go/v8"
	"github.com/aerospike/aerospike-client-go/v8/types"

	"github.com/FrogoAI/lsh/model"
)

const (
	// Bins
	binMembers   = "m" // Map<UserID, Length>
	binInput     = "i" // String
	binGroup     = "g" // String
	binSignature = "s" // String

	// It should be slightly larger than your LSH Config's MaxBucketSize.
	// If a bucket has more items than this, we return a "stub" to trigger the skip logic.
	defaultHardLimit = 10000

	// TTL Constants
	bucketTTL = 14 * 24 * 3600 // 14 Days (Seconds)
	recordTTL = 90 * 24 * 3600 // 90 Days (Seconds)
)

type Repository struct {
	client    *as.Client
	namespace string
	set       string

	writePolicy *as.WritePolicy
	readPolicy  *as.BasePolicy

	hardLimit int
}

// NewRepository creates a new Aerospike repository.
func NewRepository(client *as.Client, namespace, set string, hardLimit int) *Repository {
	if hardLimit <= 0 {
		hardLimit = defaultHardLimit
	}

	wp := as.NewWritePolicy(0, 0)
	// wp.SendKey = true // Useful for debugging/scans

	return &Repository{
		client:      client,
		namespace:   namespace,
		set:         set,
		writePolicy: wp,
		readPolicy:  as.NewPolicy(),
		hardLimit:   hardLimit,
	}
}

func (r *Repository) AddToBucket(bucketKey string, value string, length int) error {
	key, err := as.NewKey(r.namespace, r.set, bucketKey)
	if err != nil {
		return err
	}

	mapPolicy := as.NewMapPolicy(as.MapOrder.UNORDERED, as.MapWriteMode.UPDATE)

	op := as.MapPutOp(mapPolicy, binMembers, value, length)

	wp := *r.writePolicy
	wp.Expiration = bucketTTL

	_, err = r.client.Operate(&wp, key, op)

	return err
}

func (r *Repository) GetBucketMembers(bucketKey string) ([]string, []int, error) {
	key, err := as.NewKey(r.namespace, r.set, bucketKey)
	if err != nil {
		return nil, nil, err
	}

	opSize := as.MapSizeOp(binMembers)

	record, err := r.client.Operate(r.writePolicy, key, opSize)
	if err != nil {
		if err.Matches(types.KEY_NOT_FOUND_ERROR) {
			return []string{}, []int{}, nil
		}

		return nil, nil, err
	}

	sizeInt := 0
	if res, ok := record.Bins[binMembers].(int); ok {
		sizeInt = res
	} else {
		return []string{}, []int{}, nil
	}

	if sizeInt > r.hardLimit {
		return make([]string, sizeInt), make([]int, sizeInt), nil
	}

	fullRecord, err := r.client.Get(r.readPolicy, key, binMembers)
	if err != nil {
		return nil, nil, err
	}

	rawMap, ok := fullRecord.Bins[binMembers].(map[interface{}]interface{})
	if !ok {
		return []string{}, []int{}, nil
	}

	ids := make([]string, 0, len(rawMap))
	lengths := make([]int, 0, len(rawMap))

	for k, v := range rawMap {
		if idStr, ok := k.(string); ok {
			var lenVal int

			switch n := v.(type) {
			case int:
				lenVal = n
			case int64:
				lenVal = int(n)
			case float64:
				lenVal = int(n)
			}

			ids = append(ids, idStr)
			lengths = append(lengths, lenVal)
		}
	}

	return ids, lengths, nil
}

func (r *Repository) SaveRecord(u model.Record) error {
	key, err := as.NewKey(r.namespace, r.set, u.ID)
	if err != nil {
		return err
	}

	bins := as.BinMap{
		binInput:     u.Input,
		binGroup:     u.GroupID,
		binSignature: u.Signature,
	}

	wp := *r.writePolicy
	wp.Expiration = recordTTL

	return r.client.Put(&wp, key, bins)
}

func (r *Repository) GetRecords(userIDs []string) (map[string]model.Record, error) {
	if len(userIDs) == 0 {
		return map[string]model.Record{}, nil
	}

	keys := make([]*as.Key, len(userIDs))
	for i, id := range userIDs {
		k, err := as.NewKey(r.namespace, r.set, id)
		if err != nil {
			return nil, err
		}

		keys[i] = k
	}

	records, err := r.client.BatchGet(as.NewBatchPolicy(), keys, binInput, binGroup, binSignature)
	if err != nil {
		return nil, err
	}

	// Map Results
	results := make(map[string]model.Record, len(userIDs))

	for i, rec := range records {
		if rec != nil {
			// Safe type assertions
			input, _ := rec.Bins[binInput].(string)
			group, _ := rec.Bins[binGroup].(string)

			rawSig, _ := rec.Bins[binSignature].([]interface{})

			sig := make([]uint64, len(rawSig))
			for i, v := range rawSig {
				switch n := v.(type) {
				case int:
					sig[i] = uint64(n)
				case int64:
					sig[i] = uint64(n)
				case float64:
					sig[i] = uint64(n)
				}
			}

			results[userIDs[i]] = model.Record{
				ID:        userIDs[i],
				Input:     input,
				GroupID:   group,
				Signature: sig,
			}
		}
	}

	return results, nil
}

func (r *Repository) Close() {
	if r.client != nil {
		r.client.Close()
	}
}

func (r *Repository) BatchAddToBuckets(bucketKeys []string, value string, length int) error {
	if len(bucketKeys) == 0 {
		return nil
	}

	mapPolicy := as.NewMapPolicy(as.MapOrder.UNORDERED, as.MapWriteMode.UPDATE)
	op := as.MapPutOp(mapPolicy, binMembers, value, length)

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

	err := r.client.BatchOperate(nil, records)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) BatchGetBuckets(bucketKeys []string) (map[string][]string, map[string][]int, error) {
	keys := make([]*as.Key, len(bucketKeys))
	for i, k := range bucketKeys {
		key, err := as.NewKey(r.namespace, r.set, k)
		if err != nil {
			return nil, nil, err
		}

		keys[i] = key
	}

	records, err := r.client.BatchGet(nil, keys, binMembers)
	if err != nil {
		return nil, nil, err
	}

	allMembers := make(map[string][]string)
	allLens := make(map[string][]int)

	for i, rec := range records {
		if rec == nil {
			continue
		}

		if rawMap, ok := rec.Bins[binMembers].(map[interface{}]interface{}); ok {
			bKey := bucketKeys[i]
			for id, l := range rawMap {
				idStr, ok := id.(string)
				if !ok {
					continue
				}

				var lenVal int

				switch n := l.(type) {
				case int:
					lenVal = n
				case int64:
					lenVal = int(n)
				case float64:
					lenVal = int(n)
				}

				allMembers[bKey] = append(allMembers[bKey], idStr)
				allLens[bKey] = append(allLens[bKey], lenVal)
			}
		}
	}

	return allMembers, allLens, nil
}
