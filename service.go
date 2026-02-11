package lsh

import (
	"context"
	"encoding/hex"
	"sync"

	"github.com/FrogoAI/lsh/model"
	"github.com/FrogoAI/lsh/repositories"

	"github.com/FrogoAI/multiproc/worker"
	"github.com/FrogoAI/set"
)

var signaturePool = sync.Pool{
	New: func() interface{} {
		s := make([]uint64, SignatureSize)
		return &s
	},
}

type SimilarityService struct {
	hasher *Hasher
	repo   repositories.Storage
	config *Config
}

func NewSimilarityService(repo repositories.Storage, config *Config) *SimilarityService {
	return &SimilarityService{
		hasher: NewHasher(config.Bands, config.Rows, config.Seed),
		repo:   repo,
		config: config,
	}
}

func (s *SimilarityService) GetNewID() (string, error) {
	id, err := GetTinyID()
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(id), nil
}

func (s *SimilarityService) GetPartition(group, key string) (string, error) {
	prefix, err := s.config.HashVersion(group)
	if err != nil {
		return "", err
	}

	return prefix + ":" + key, nil
}

func (s *SimilarityService) Upsert(ctx context.Context, group, input string) (string, error) {
	// maximum amount of allocation decreased to amount of concurrent processes
	sigPtr := signaturePool.Get().(*[]uint64)
	defer signaturePool.Put(sigPtr)
	sig := *sigPtr

	inputTokens := s.Shingle(input)

	inputLen := len(inputTokens)
	if inputLen == 0 {
		return "", ErrShingleResultIsEmpty
	}

	s.hasher.ComputeSignature(inputTokens, sig)

	// Calculate (for optimization purpose) min and max len of diff between input and result
	minLen := int(float64(inputLen) * s.config.JaccardThreshold)
	maxLen := int(float64(inputLen) / s.config.JaccardThreshold)

	keys, err := s.hasher.ComputeBands(sig)
	if err != nil {
		return "", err
	}

	candidateSet := set.NewGenericDataSet[string]()

	for _, key := range keys {
		if candidateSet.Count() >= s.config.MaxTotalCandidates {
			// If we take too many candidates skip selecting more
			break
		}

		// we make partition to separate keys by group and unique config hash
		partitionKey, err := s.GetPartition(group, key)
		if err != nil {
			return "", err
		}

		members, lenghts, err := s.repo.GetBucketMembers(partitionKey)
		if err != nil {
			return "", err
		}

		if len(members) > s.config.MaxBucketSize {
			// That can cause decreasing of accuracy, but if bucket contains a
			// lot of records (hot bucket), we must skip it
			continue
		}

		for i, id := range members {
			cLen := lenghts[i]

			// Length Filter, we do not care about Jaccard calculation if size too different
			if cLen < minLen || cLen > maxLen {
				continue
			}

			candidateSet.Add(id)
		}
	}

	if candidateSet.Count() > 0 {
		ids := candidateSet.ToSlice()

		profiles, err := s.repo.GetRecords(ids)
		if err != nil {
			return "", err
		}

		sourceSet := set.NewGenericDataSet[string](inputTokens...)

		for _, p := range profiles {
			score := s.CalculateJaccardOptimized(sourceSet, p.Input)
			if score >= s.config.JaccardThreshold {
				return p.ID, nil
			}
		}
	}

	// If we not found bucket, we create new one
	bid, err := s.GetNewID()
	if err != nil {
		return "", err
	}

	err = s.repo.SaveRecord(model.Record{
		ID:      bid,
		Input:   input,
		GroupID: group,
	})
	if err != nil {
		return "", err
	}

	pool := worker.NewPool(ctx)

	for _, key := range keys {
		pool.Execute(func(ctx context.Context) error {
			partitionKey, err := s.GetPartition(group, key)
			if err != nil {
				return err
			}

			if err := s.repo.AddToBucket(partitionKey, bid, inputLen); err != nil {
				return err
			}

			return nil
		})
	}

	return "", pool.Wait()
}
