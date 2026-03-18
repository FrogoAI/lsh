package vector

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"log/slog"
	"math"
	"sync"

	"github.com/FrogoAI/lsh/v2"
	"github.com/FrogoAI/lsh/v2/repositories"
	"github.com/FrogoAI/multiproc/worker"
	"github.com/FrogoAI/set"
)

const cosineMargin = 0.1

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
		hasher:        NewHasher(config.Bands, config.Rows, config.VectorDimensions, config.Seed),
		repo:          repo,
		config:        config,
		signaturePool: lsh.NewSignaturePool(config.SignatureSize()),
	}
}

// GetNewID returns a deterministic ID derived from the vector content.
func (s *Service) GetNewID(vector []float64) string {
	buf := make([]byte, len(vector)*8) //nolint:mnd

	for i, v := range vector {
		binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(v)) //nolint:mnd
	}

	hash := sha256.Sum256(buf)

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

// Upsert indexes a vector and returns a behavioural ID.
// If a similar vector already exists (cosine >= threshold), returns the existing ID.
// Otherwise stores the vector and returns a new ID.
func (s *Service) Upsert(ctx context.Context, group string, vector []float64) (string, error) {
	if len(vector) == 0 {
		return "", ErrEmptyVector
	}

	if len(vector) != s.config.VectorDimensions {
		return "", ErrWrongDimension
	}

	bid := s.GetNewID(vector)

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

	s.hasher.ComputeSignature(vector, sig)

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

			estCosine := EstimateCosine(sig, p.Signature)

			if estCosine < (s.config.CosineThreshold - cosineMargin) {
				continue
			}

			score := ExactCosine(vector, p.Vector)
			if score >= s.config.CosineThreshold {
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
			Vector:    vector,
			GroupID:   group,
			Signature: sigCopy,
		}

		return s.repo.SaveRecord(bid, rec.toBins())
	})

	pool.Execute(func(_ context.Context) error {
		return s.repo.BatchSetRepresentative(bucketKeys, bid, 0)
	})

	return bid, pool.Wait()
}
