package aerospike

import (
	"github.com/FrogoAI/lsh/model"
	as "github.com/aerospike/aerospike-client-go/v7"
	"github.com/aerospike/aerospike-client-go/v7/types"
)

type Repository struct {
	client    *as.Client
	namespace string
	set       string
}

const (
	BinMembers = "m" // List of strings (ID:Length)
	BinInput   = "i" // Original Input String
)

func NewRepository(host string, port int, ns, set string) (*Repository, error) {
	policy := as.NewClientPolicy()
	policy.ConnectionQueueSize = 100
	policy.LimitConnectionsToQueueSize = true

	client, err := as.NewClientWithPolicy(policy, host, port)
	if err != nil {
		return nil, err
	}
	return &Repository{client: client, namespace: ns, set: set}, nil
}

func (r *Repository) AddToBucket(bucketKey string, value string) error {
	key, _ := as.NewKey(r.namespace, r.set, bucketKey)
	listPolicy := as.NewListPolicy(as.ListOrderUnordered, as.ListWriteFlagsDefault)
	ops := []*as.Operation{as.ListAppendWithPolicyOp(listPolicy, BinMembers, value)}
	_, err := r.client.Operate(nil, key, ops...)
	return err
}

func (r *Repository) GetBucketMembers(bucketKey string) ([]string, error) {
	key, _ := as.NewKey(r.namespace, r.set, bucketKey)
	rec, err := r.client.Get(nil, key, BinMembers)
	if err != nil {
		if err.Matches(types.KEY_NOT_FOUND_ERROR) {
			return nil, nil
		}
		return nil, err
	}
	if raw, ok := rec.Bins[BinMembers].([]interface{}); ok {
		res := make([]string, len(raw))
		for i, v := range raw {
			res[i] = v.(string)
		}
		return res, nil
	}
	return nil, nil
}

func (r *Repository) SaveRecord(u model.Record) error {
	key, _ := as.NewKey(r.namespace, r.set, u.ID)
	bins := as.BinMap{BinInput: u.Input}
	return r.client.Put(nil, key, bins)
}

func (r *Repository) GetRecords(userIDs []string) (map[string]model.Record, error) {
	if len(userIDs) == 0 {
		return map[string]model.Record{}, nil
	}

	keys := make([]*as.Key, len(userIDs))
	for i, id := range userIDs {
		keys[i], _ = as.NewKey(r.namespace, r.set, id)
	}

	records, err := r.client.BatchGet(nil, keys, BinInput)
	if err != nil {
		return nil, err
	}

	result := make(map[string]model.Record, len(userIDs))
	for i, rec := range records {
		if rec == nil {
			continue
		}
		if input, ok := rec.Bins[BinInput].(string); ok {
			result[userIDs[i]] = model.Record{ID: userIDs[i], Input: input}
		}
	}
	return result, nil
}

func (r *Repository) Close() { r.client.Close() }
