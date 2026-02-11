package repositories

import "github.com/FrogoAI/lsh/model"

type Storage interface {
	AddToBucket(bucketKey string, value string, len int) error
	GetBucketMembers(bucketKey string) ([]string, []int, error)
	SaveRecord(u model.Record) error
	GetRecords(userIDs []string) (map[string]model.Record, error)
	Close()
}
