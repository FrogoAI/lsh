package lsh

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"hash/fnv"
	"log/slog"
	"sync"

	"github.com/FrogoAI/lsh/model"
	"github.com/FrogoAI/lsh/repositories"
	"github.com/FrogoAI/multiproc/worker"
	"github.com/FrogoAI/set"
)

const (
	jaccardThreshold = 0.1
	groupLockShards  = 64
)

type SimilarityService struct {
	hasher        *Hasher
	repo          repositories.Storage
	config        *Config
	signaturePool *sync.Pool
	groupLocks    [groupLockShards]sync.Mutex
	prefixCache   sync.Map // group string -> prefix string
	resolvedCache sync.Map // bid (string) -> resolved bid (string)
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

func (s *SimilarityService) GetNewID(input string) string {
	hash := sha256.Sum256([]byte(input))

	return base64.RawURLEncoding.EncodeToString(hash[:16])
}

func (s *SimilarityService) getPrefix(group string) (string, error) {
	if v, ok := s.prefixCache.Load(group); ok {
		return v.(string), nil
	}

	prefix, err := s.config.HashVersion(group)
	if err != nil {
		return "", err
	}

	s.prefixCache.Store(group, prefix)

	return prefix, nil
}

func (s *SimilarityService) groupShard(group string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(group)) //nolint:errcheck

	return h.Sum32() % groupLockShards
}

func (s *SimilarityService) Upsert(ctx context.Context, group, input string) (string, error) {
	if input == "" {
		return "", ErrEmptyInputString
	}

	bid := s.GetNewID(input)

	existing, err := s.repo.GetRecords([]string{bid})
	if err != nil {
		return "", err
	}

	if _, ok := existing[bid]; ok {
		return bid, nil
	}

	// L1: in-memory cache (per-pod, instant)
	if resolved, ok := s.resolvedCache.Load(bid); ok {
		return resolved.(string), nil
	}

	// L2: persistent cache (shared across pods, survives restarts)
	if resolved, err := s.repo.GetResolvedID(bid); err != nil {
		return "", err
	} else if resolved != "" {
		s.resolvedCache.Store(bid, resolved)

		return resolved, nil
	}

	shard := s.groupShard(group)
	s.groupLocks[shard].Lock()
	defer s.groupLocks[shard].Unlock()

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

	prefix, err := s.getPrefix(group)
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

	candidateSet := set.NewGenericDataSet[string]()

	for _, bk := range bucketKeys {
		if candidateSet.Count() >= s.config.MaxTotalCandidates {
			break
		}

		members, lengths := allMembers[bk], allLens[bk]

		if len(members) > s.config.MaxBucketSize {
			continue
		}

		for i, id := range members {
			cLen := lengths[i]

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

		// Iterate in deterministic order to ensure consistent results
		for _, id := range ids {
			p, ok := profiles[id]
			if !ok {
				continue
			}

			estJaccard := EstimateJaccard(sig, p.Signature)

			if estJaccard < (s.config.JaccardThreshold - jaccardThreshold) {
				continue
			}

			score := s.CalculateJaccardOptimized(inputTokens, p.Input)
			if score >= s.config.JaccardThreshold {
				s.resolvedCache.Store(bid, p.ID)

				if err := s.repo.SaveResolvedID(bid, p.ID); err != nil {
					slog.Warn("failed to persist resolved ID to L2 cache",
						slog.String("bid", bid),
						slog.String("resolved", p.ID),
						slog.Any("error", err),
					)
				}

				return p.ID, nil
			}
		}
	}

	pool := worker.NewPool(ctx)

	pool.Execute(func(_ context.Context) error {
		sigCopy := make([]uint64, len(sig))
		copy(sigCopy, sig)

		return s.repo.SaveRecord(model.Record{
			ID:        bid,
			Input:     input,
			GroupID:   group,
			Signature: sigCopy,
		})
	})

	pool.Execute(func(_ context.Context) error {
		return s.repo.BatchAddToBuckets(bucketKeys, bid, inputLen)
	})

	return bid, pool.Wait()
}
