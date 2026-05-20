package dedup

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"sort"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"go.opentelemetry.io/otel/metric"

	"github.com/FrogoAI/lsh/v2"
	"github.com/FrogoAI/lsh/v2/repositories"
	"github.com/FrogoAI/multiproc/worker"
)

const jaccardMargin = 0.1

type Service struct {
	hasher        *Hasher
	repo          repositories.Storage
	config        *Config
	signaturePool *sync.Pool
	groupLocks    [lsh.GroupLockShards]sync.Mutex
	prefixCache   sync.Map
	resolvedCache *lru.Cache[string, string]
	metrics       *lsh.Instruments
}

func NewService(repo repositories.Storage, config *Config) (*Service, error) {
	cacheSize := config.ResolvedCacheSize
	if cacheSize <= 0 {
		cacheSize = 1 //nolint:mnd
	}

	cache, err := lru.New[string, string](cacheSize)
	if err != nil {
		return nil, err
	}

	return &Service{
		hasher:        NewHasher(config.Bands, config.Rows, config.Seed),
		repo:          repo,
		config:        config,
		signaturePool: lsh.NewSignaturePool(config.SignatureSize()),
		resolvedCache: cache,
	}, nil
}

// WithMetrics injects pre-built OpenTelemetry instruments.
// If not called, metrics are silently skipped.
func (s *Service) WithMetrics(m *lsh.Instruments) {
	s.metrics = m
}

// GetNewID returns a deterministic ID scoped by group and input content (SHA256, base64url).
func (s *Service) GetNewID(group, input string) string {
	buf := make([]byte, 0, len(group)+1+len(input))
	buf = append(buf, group...)
	buf = append(buf, 0)
	buf = append(buf, input...)

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

func (s *Service) Upsert(ctx context.Context, group, input string) (string, error) {
	start := time.Now()

	if input == "" {
		return "", ErrEmptyInputString
	}

	bid := s.GetNewID(group, input)

	existing, err := s.repo.GetRecords([]string{bid})
	if err != nil {
		return "", err
	}

	for _, rec := range existing {
		if rec.Key == bid {
			s.recordUpsert(ctx, start, lsh.ResultL1Hit, group)

			return bid, nil
		}
	}

	if resolved, ok := s.resolvedCache.Get(bid); ok {
		s.recordUpsert(ctx, start, lsh.ResultL2Hit, group)

		return resolved, nil
	}

	if resolved, err := s.repo.GetValue("res:" + bid); err != nil {
		return "", err
	} else if resolved != "" {
		s.resolvedCache.Add(bid, resolved)
		s.recordUpsert(ctx, start, lsh.ResultL3Hit, group)

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

	ids := s.collectCandidates(bucketKeys, allReps, int64(minLen), int64(maxLen))

	s.recordCandidateStats(ctx, bucketKeys, allReps, ids)

	if len(ids) > 0 {
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

		exactChecks := 0

		for _, id := range ids {
			p, ok := profiles[id]
			if !ok {
				continue
			}

			estJaccard := EstimateJaccard(sig, p.Signature)

			if estJaccard < (s.config.JaccardThreshold - jaccardMargin) {
				continue
			}

			exactChecks++

			score := CalculateJaccardOptimized(inputTokens, p.Input, s.config.ShingleSize)
			if score >= s.config.JaccardThreshold {
				s.resolvedCache.Add(bid, p.ID)

				if err := s.repo.PutValue("res:"+bid, p.ID); err != nil {
					slog.Warn("failed to persist resolved ID to L2 cache",
						slog.String("bid", bid),
						slog.String("resolved", p.ID),
						slog.Any("error", err),
					)
				}

				s.recordExactChecks(ctx, exactChecks)
				s.recordUpsert(ctx, start, lsh.ResultMatch, group)

				return p.ID, nil
			}
		}

		s.recordExactChecks(ctx, exactChecks)
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

	err = pool.Wait()

	s.recordUpsert(ctx, start, lsh.ResultNew, group)

	if s.metrics != nil {
		s.metrics.NewIDTotal.Add(ctx, 1, metric.WithAttributes(lsh.AttrGroup.String(group)))
	}

	return bid, err
}

// collectCandidates scans all bands, counts band overlap per candidate,
// filters by metadata (shingle length), and returns the top MaxTotalCandidates
// sorted by overlap count (descending).
func (s *Service) collectCandidates(
	bucketKeys []string,
	allReps map[string][]repositories.Representative,
	minMeta, maxMeta int64,
) []string {
	bandCount := make(map[string]int)

	for _, bk := range bucketKeys {
		for _, rep := range allReps[bk] {
			if rep.Metadata < minMeta || rep.Metadata > maxMeta {
				continue
			}

			bandCount[rep.ID]++
		}
	}

	if len(bandCount) == 0 {
		return nil
	}

	type ranked struct {
		id    string
		bands int
	}

	candidates := make([]ranked, 0, len(bandCount))
	for id, count := range bandCount {
		candidates = append(candidates, ranked{id: id, bands: count})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].bands > candidates[j].bands
	})

	limit := len(candidates)
	if s.config.MaxTotalCandidates > 0 && limit > s.config.MaxTotalCandidates {
		limit = s.config.MaxTotalCandidates
	}

	ids := make([]string, limit)
	for i := 0; i < limit; i++ {
		ids[i] = candidates[i].id
	}

	return ids
}

func (s *Service) recordUpsert(ctx context.Context, start time.Time, result, group string) {
	if s.metrics == nil {
		return
	}

	attrs := metric.WithAttributes(lsh.AttrResult.String(result), lsh.AttrGroup.String(group))

	s.metrics.UpsertDuration.Record(ctx, time.Since(start).Seconds(), attrs)
	s.metrics.UpsertTotal.Add(ctx, 1, attrs)
}

func (s *Service) recordCandidateStats(
	ctx context.Context,
	bucketKeys []string,
	allReps map[string][]repositories.Representative,
	ids []string,
) {
	if s.metrics == nil {
		return
	}

	totalReps := 0
	for _, bk := range bucketKeys {
		totalReps += len(allReps[bk])
	}

	s.metrics.BucketRepsReturned.Record(ctx, int64(totalReps))
	s.metrics.CandidateCount.Record(ctx, int64(len(ids)))
}

func (s *Service) recordExactChecks(ctx context.Context, count int) {
	if s.metrics == nil {
		return
	}

	s.metrics.ExactCompareCount.Record(ctx, int64(count))
}
