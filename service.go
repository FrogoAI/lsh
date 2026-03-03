package lsh

import (
	"context"
	"strconv"
	"sync"

	"github.com/mfonda/simhash"

	"github.com/FrogoAI/lsh/model"
	"github.com/FrogoAI/lsh/repositories"
	"github.com/FrogoAI/multiproc/worker"
	"github.com/FrogoAI/set"
)

const jaccardThreshold = 0.1

type SimilarityService struct {
	hasher        *Hasher
	repo          repositories.Storage
	config        *Config
	signaturePool *sync.Pool
}

func NewSimilarityService(repo repositories.Storage, config *Config) *SimilarityService {
	sigSize := config.Bands * config.Rows

	svc := &SimilarityService{
		hasher: NewHasher(config.Bands, config.Rows, config.Seed),
		repo:   repo,
		config: config,
		signaturePool: &sync.Pool{
			New: func() interface{} {
				// Allocate EXACTLY what Bands * Rows needs
				slice := make([]uint64, sigSize)
				return &slice
			},
		},
	}

	return svc
}

func (s *SimilarityService) GetNewID(input string) (string, error) {
	value := simhash.Simhash(simhash.NewWordFeatureSet([]byte(input)))
	hexString := strconv.FormatUint(value, 16)

	return hexString, nil
}

func (s *SimilarityService) Upsert(ctx context.Context, group, input string) (string, error) {
	// maximum amount of allocation decreased to amount of concurrent processes
	sigPtr := s.signaturePool.Get().(*[]uint64)
	defer s.signaturePool.Put(sigPtr)

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

	prefix, err := s.config.HashVersion(group)
	if err != nil {
		return "", err
	}

	bucketKeys := make([]string, len(keys))
	for i, k := range keys {
		bucketKeys[i] = prefix + ":" + k
	}

	allMembers, allLens, err := s.repo.BatchGetBuckets(bucketKeys)
	if err != nil {
		return "", err
	}

	for _, key := range keys {
		if candidateSet.Count() >= s.config.MaxTotalCandidates {
			// If we take too many candidates skip selecting more
			break
		}

		// we make partition to separate keys by group and unique config hash
		partitionKey := prefix + ":" + key

		members, lengths := allMembers[partitionKey], allLens[partitionKey]

		if len(members) > s.config.MaxBucketSize {
			// That can cause decreasing of accuracy, but if bucket contains a
			// lot of records (hot bucket), we must skip it
			continue
		}

		for i, id := range members {
			cLen := lengths[i]

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

		for _, p := range profiles {
			estJaccard := EstimateJaccard(sig, p.Signature)

			if estJaccard < (s.config.JaccardThreshold - jaccardThreshold) {
				continue // Skip expensive text processing for this candidate
			}

			score := s.CalculateJaccardOptimized(inputTokens, p.Input)
			if score >= s.config.JaccardThreshold {
				return p.ID, nil
			}
		}
	}

	// If we not found bucket, we create new one
	bid, err := s.GetNewID(input)
	if err != nil {
		return "", err
	}

	pool := worker.NewPool(ctx)

	pool.Execute(func(_ context.Context) error {
		return s.repo.SaveRecord(model.Record{
			ID:        bid,
			Input:     input,
			GroupID:   group,
			Signature: sig,
		})
	})

	pool.Execute(func(_ context.Context) error {
		partitionedKeys := make([]string, len(keys))
		for i, k := range keys {
			partitionedKeys[i] = prefix + ":" + k
		}

		return s.repo.BatchAddToBuckets(partitionedKeys, bid, inputLen)
	})

	return bid, pool.Wait()
}
