package dedup

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"sync"

	"github.com/FrogoAI/lsh/v2"
	"github.com/FrogoAI/lsh/v2/repositories"
	"github.com/FrogoAI/multiproc/worker"
	"github.com/FrogoAI/set"
)

const jaccardMargin = 0.1

type Service struct {
	hasher        *Hasher
	repo          repositories.Storage
	config        *Config
	signaturePool *sync.Pool
	groupLocks    [lsh.GroupLockShards]sync.Mutex
	prefixCache   sync.Map
	resolvedCache sync.Map
}

func NewService(repo repositories.Storage, config *Config) *Service {
	return &Service{
		hasher:        NewHasher(config.Bands, config.Rows, config.Seed),
		repo:          repo,
		config:        config,
		signaturePool: lsh.NewSignaturePool(config.SignatureSize()),
	}
}

func (s *Service) GetNewID(input string) string {
	hash := sha256.Sum256([]byte(input))

	return base64.RawURLEncoding.EncodeToString(hash[:16])
}

func (s *Service) getPrefix(group string) (string, error) {
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

func (s *Service) Upsert(ctx context.Context, group, input string) (string, error) {
	if input == "" {
		return "", ErrEmptyInputString
	}

	bid := s.GetNewID(input)

	existing, err := s.repo.GetRecords([]string{bid})
	if err != nil {
		return "", err
	}

	for _, rec := range existing {
		if rec.Key == bid {
			return bid, nil
		}
	}

	if resolved, ok := s.resolvedCache.Load(bid); ok {
		return resolved.(string), nil
	}

	if resolved, err := s.repo.GetValue("res:" + bid); err != nil {
		return "", err
	} else if resolved != "" {
		s.resolvedCache.Store(bid, resolved)

		return resolved, nil
	}

	shard := lsh.GroupShard(group)
	s.groupLocks[shard].Lock()
	defer s.groupLocks[shard].Unlock()

	sigPtr := s.signaturePool.Get().(*[]uint64)
	defer s.signaturePool.Put(sigPtr)

	sig := *sigPtr

	inputTokens := Shingle(input, s.config.ShingleSize)

	inputLen := len(inputTokens)
	if inputLen == 0 {
		return "", ErrShingleResultIsEmpty
	}

	s.hasher.ComputeSignature(inputTokens, sig)

	minLen := int(float64(inputLen) * s.config.JaccardThreshold)
	maxLen := int(float64(inputLen) / s.config.JaccardThreshold)

	keys, err := lsh.ComputeBands(sig, s.config.Bands, s.config.Rows)
	if err != nil {
		return "", err
	}

	prefix, err := s.getPrefix(group)
	if err != nil {
		return "", err
	}

	bucketKeys := lsh.PrefixKeys(prefix, keys)

	allReps, err := s.repo.BatchGetRepresentatives(bucketKeys)
	if err != nil {
		return "", err
	}

	candidateSet := set.NewGenericDataSet[string]()

	for _, bk := range bucketKeys {
		if candidateSet.Count() >= s.config.MaxTotalCandidates {
			break
		}

		for _, rep := range allReps[bk] {
			if rep.Metadata < int64(minLen) || rep.Metadata > int64(maxLen) {
				continue
			}

			candidateSet.Add(rep.ID)
		}
	}

	if candidateSet.Count() > 0 {
		ids := candidateSet.ToSlice()

		rawRecords, err := s.repo.GetRecords(ids)
		if err != nil {
			return "", err
		}

		profiles := make(map[string]Record, len(rawRecords))
		for _, raw := range rawRecords {
			if r, ok := recordFromBins(raw); ok {
				profiles[r.ID] = r
			}
		}

		for _, id := range ids {
			p, ok := profiles[id]
			if !ok {
				continue
			}

			estJaccard := EstimateJaccard(sig, p.Signature)

			if estJaccard < (s.config.JaccardThreshold - jaccardMargin) {
				continue
			}

			score := CalculateJaccardOptimized(inputTokens, p.Input, s.config.ShingleSize)
			if score >= s.config.JaccardThreshold {
				s.resolvedCache.Store(bid, p.ID)

				if err := s.repo.PutValue("res:"+bid, p.ID); err != nil {
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

		rec := Record{
			ID:        bid,
			Input:     input,
			GroupID:   group,
			Signature: sigCopy,
		}

		return s.repo.SaveRecord(bid, rec.toBins())
	})

	pool.Execute(func(_ context.Context) error {
		return s.repo.BatchSetRepresentative(bucketKeys, bid, int64(inputLen))
	})

	return bid, pool.Wait()
}
