package aerospike

import (
	as "github.com/aerospike/aerospike-client-go/v7"
	"github.com/aerospike/aerospike-client-go/v7/types"

	"github.com/FrogoAI/lsh/model"
)

const (
	// Bins
	binMembers = "m" // Map<UserID, Length>
	binInput   = "i" // String
	binGroup   = "g" // String

	// Configuration
	// This hard limit protects the application from OOMing on massive buckets.
	// It should be slightly larger than your LSH Config's MaxBucketSize.
	// If a bucket has more items than this, we return a "stub" to trigger the skip logic.
	defaultHardLimit = 5000

	// TTL Constants
	bucketTTL = 14 * 24 * 3600 // 14 Days (Seconds)
	recordTTL = 90 * 24 * 3600 // 90 Days (Seconds)
)

type Repository struct {
	client    *as.Client
	namespace string
	set       string

	// Policies
	writePolicy *as.WritePolicy
	readPolicy  *as.BasePolicy

	// Safety
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

	// STEP 1: Peek at the Size (Server-Side)
	// We operate on the 'm' bin to get COUNT, returning an integer.
	opSize := as.MapSizeOp(binMembers)

	record, err := r.client.Operate(r.writePolicy, key, opSize)
	if err != nil {
		if err.Matches(types.KEY_NOT_FOUND_ERROR) {
			return []string{}, []int{}, nil
		}
		return nil, nil, err
	}

	// Parse Size
	sizeInt := 0
	if res, ok := record.Bins[binMembers].(int); ok {
		sizeInt = res
	} else {
		// Edge case: Map is empty or nil
		return []string{}, []int{}, nil
	}

	// STEP 2: The Safety Valve (Stub Return)
	if sizeInt > r.hardLimit {
		// Optimization: Return a slice of correct LENGTH but empty CONTENT.
		// The LSH service checks 'if len > MaxBucketSize' and continues.
		// We save network bandwidth and memory allocation.
		return make([]string, sizeInt), make([]int, sizeInt), nil
	}

	// STEP 3: Safe Fetch
	// The bucket is within limits. Fetch the actual map.
	fullRecord, err := r.client.Get(r.readPolicy, key, binMembers)
	if err != nil {
		return nil, nil, err
	}

	// Parse Map (CDT)
	// Aerospike returns Map bins as map[interface{}]interface{}
	rawMap, ok := fullRecord.Bins[binMembers].(map[interface{}]interface{})
	if !ok {
		return []string{}, []int{}, nil
	}

	ids := make([]string, 0, len(rawMap))
	lengths := make([]int, 0, len(rawMap))

	for k, v := range rawMap {
		if idStr, ok := k.(string); ok {
			// Handle numeric type variance in Aerospike client
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
		binInput: u.Input,
		binGroup: u.GroupID,
	}

	// Copy policy to set specific TTL for records
	wp := *r.writePolicy
	wp.Expiration = recordTTL

	return r.client.Put(&wp, key, bins)
}

func (r *Repository) GetRecords(userIDs []string) (map[string]model.Record, error) {
	if len(userIDs) == 0 {
		return map[string]model.Record{}, nil
	}

	// Prepare Batch Keys
	keys := make([]*as.Key, len(userIDs))
	for i, id := range userIDs {
		k, err := as.NewKey(r.namespace, r.set, id)
		if err != nil {
			return nil, err
		}
		keys[i] = k
	}

	// Execute Batch
	// nil for binNames means "fetch all bins"
	records, err := r.client.BatchGet(as.NewBatchPolicy(), keys, binInput, binGroup)
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

			results[userIDs[i]] = model.Record{
				ID:      userIDs[i],
				Input:   input,
				GroupID: group,
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
