package repositories

import "github.com/FrogoAI/lsh/model"

type Storage interface {
	AddToBucket(bucketKey string, value string, l int) error
	GetBucketMembers(bucketKey string) ([]string, []int, error)
	SaveRecord(u model.Record) error
	GetRecords(userIDs []string) (map[string]model.Record, error)
	BatchAddToBuckets(bucketKeys []string, value string, length int) error
	BatchGetBuckets(bucketKeys []string) (map[string][]string, map[string][]int, error)
	SaveResolvedID(bid string, resolvedBid string) error
	GetResolvedID(bid string) (string, error)
	Close()
}
