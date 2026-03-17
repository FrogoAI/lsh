package vector

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/FrogoAI/lsh"
	"github.com/FrogoAI/lsh/repositories"
	"github.com/FrogoAI/multiproc/worker"
	"github.com/FrogoAI/set"
)

const cosineMargin = 0.1

type Match struct {
	UserID     string
	Similarity float64
}

type Service struct {
	hasher        *Hasher
	repo          repositories.Storage
	config        *Config
	signaturePool *sync.Pool
	groupLocks    [lsh.GroupLockShards]sync.Mutex
	prefixCache   sync.Map
}

func NewService(repo repositories.Storage, config *Config) *Service {
	return &Service{
		hasher:        NewHasher(config.Bands, config.Rows, config.VectorDimensions, config.Seed),
		repo:          repo,
		config:        config,
		signaturePool: lsh.NewSignaturePool(config.SignatureSize()),
	}
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

// Index adds or updates a user's vector in the LSH index.
func (s *Service) Index(ctx context.Context, group, userID string, vector []float64) error {
	if len(vector) == 0 {
		return ErrEmptyVector
	}

	if len(vector) != s.config.VectorDimensions {
		return ErrWrongDimension
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
		return err
	}

	prefix, err := s.getPrefix(group)
	if err != nil {
		return err
	}

	bucketKeys := lsh.PrefixKeys(prefix, keys)

	pool := worker.NewPool(ctx)

	pool.Execute(func(_ context.Context) error {
		sigCopy := make([]uint64, len(sig))
		copy(sigCopy, sig)

		rec := Record{
			ID:        userID,
			Vector:    vector,
			GroupID:   group,
			Signature: sigCopy,
		}

		return s.repo.SaveRecord(userID, rec.toBins())
	})

	pool.Execute(func(_ context.Context) error {
		return s.repo.BatchAddBucketMember(bucketKeys, userID, time.Now().Unix())
	})

	return pool.Wait()
}

// FindSimilar returns users with vectors similar to the query vector,
// verified by exact cosine similarity, sorted descending by score.
func (s *Service) FindSimilar(ctx context.Context, group string, vector []float64, topK int) ([]Match, error) {
	if len(vector) == 0 {
		return nil, ErrEmptyVector
	}

	if len(vector) != s.config.VectorDimensions {
		return nil, ErrWrongDimension
	}

	sigPtr := s.signaturePool.Get().(*[]uint64)
	defer s.signaturePool.Put(sigPtr)

	sig := *sigPtr

	s.hasher.ComputeSignature(vector, sig)

	keys, err := lsh.ComputeBands(sig, s.config.Bands, s.config.Rows)
	if err != nil {
		return nil, err
	}

	prefix, err := s.getPrefix(group)
	if err != nil {
		return nil, err
	}

	bucketKeys := lsh.PrefixKeys(prefix, keys)

	allMembers, err := s.repo.BatchGetBucketMembers(bucketKeys)
	if err != nil {
		return nil, err
	}

	candidateSet := set.NewGenericDataSet[string]()

	for _, bk := range bucketKeys {
		if candidateSet.Count() >= s.config.MaxTotalCandidates {
			break
		}

		members := allMembers[bk]
		if len(members) > s.config.MaxBucketSize {
			continue
		}

		for _, m := range members {
			candidateSet.Add(m.ID)
		}
	}

	if candidateSet.Count() == 0 {
		return nil, nil
	}

	ids := candidateSet.ToSlice()

	rawRecords, err := s.repo.GetRecords(ids)
	if err != nil {
		return nil, err
	}

	_ = ctx

	var matches []Match

	for _, raw := range rawRecords {
		rec, ok := recordFromBins(raw)
		if !ok {
			continue
		}

		est := EstimateCosine(sig, rec.Signature)
		if est < (s.config.CosineThreshold - cosineMargin) {
			continue
		}

		exact := ExactCosine(vector, rec.Vector)
		if exact >= s.config.CosineThreshold {
			matches = append(matches, Match{UserID: rec.ID, Similarity: exact})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})

	if topK > 0 && len(matches) > topK {
		matches = matches[:topK]
	}

	return matches, nil
}

// GetBucketPeers returns all user IDs sharing at least one LSH bucket with the query vector.
// No verification step — faster than FindSimilar.
func (s *Service) GetBucketPeers(_ context.Context, group string, vector []float64) ([]string, error) {
	if len(vector) == 0 {
		return nil, ErrEmptyVector
	}

	if len(vector) != s.config.VectorDimensions {
		return nil, ErrWrongDimension
	}

	sigPtr := s.signaturePool.Get().(*[]uint64)
	defer s.signaturePool.Put(sigPtr)

	sig := *sigPtr

	s.hasher.ComputeSignature(vector, sig)

	keys, err := lsh.ComputeBands(sig, s.config.Bands, s.config.Rows)
	if err != nil {
		return nil, err
	}

	prefix, err := s.getPrefix(group)
	if err != nil {
		return nil, err
	}

	bucketKeys := lsh.PrefixKeys(prefix, keys)

	allMembers, err := s.repo.BatchGetBucketMembers(bucketKeys)
	if err != nil {
		return nil, err
	}

	peerSet := set.NewGenericDataSet[string]()

	for _, bk := range bucketKeys {
		for _, m := range allMembers[bk] {
			peerSet.Add(m.ID)
		}
	}

	return peerSet.ToSlice(), nil
}
