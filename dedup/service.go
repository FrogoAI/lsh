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

	mu      sync.RWMutex
	meter   metric.Meter
	metrics *instruments
}

type instruments struct {
	upsertDuration     metric.Float64Histogram
	upsertTotal        metric.Int64Counter
	newIDTotal         metric.Int64Counter
	candidateCount     metric.Int64Histogram
	exactCompareCount  metric.Int64Histogram
	bucketRepsReturned metric.Int64Histogram
}

func NewService(repo repositories.Storage, config *Config) *Service {
	cacheSize := config.ResolvedCacheSize
	if cacheSize <= 0 {
		cacheSize = 1 //nolint:mnd
	}

	cache, _ := lru.New[string, string](cacheSize)

	return &Service{
		hasher:        NewHasher(config.Bands, config.Rows, config.Seed),
		repo:          repo,
		config:        config,
		signaturePool: lsh.NewSignaturePool(config.SignatureSize()),
		resolvedCache: cache,
	}
}

// WithMeter sets the OpenTelemetry meter for this service.
// If not called, metrics are silently skipped.
func (s *Service) WithMeter(m metric.Meter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.meter = m
	s.metrics = nil
}

const metricPrefix = "lsh.dedup."

func (s *Service) getMetrics() *instruments {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.metrics != nil {
		return s.metrics
	}

	if s.meter == nil {
		return nil
	}

	inst, err := newInstruments(s.meter)
	if err != nil {
		slog.Warn("failed to create LSH dedup metrics", slog.Any("error", err))

		return nil
	}

	s.metrics = inst

	return inst
}

func newInstruments(m metric.Meter) (*instruments, error) {
	var (
		inst instruments
		err  error
	)

	inst.upsertDuration, err = m.Float64Histogram(metricPrefix+lsh.MetricUpsertDuration, metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	inst.upsertTotal, err = m.Int64Counter(metricPrefix + lsh.MetricUpsertTotal)
	if err != nil {
		return nil, err
	}

	inst.newIDTotal, err = m.Int64Counter(metricPrefix + lsh.MetricNewIDTotal)
	if err != nil {
		return nil, err
	}

	inst.candidateCount, err = m.Int64Histogram(metricPrefix + lsh.MetricCandidateCount)
	if err != nil {
		return nil, err
	}

	inst.exactCompareCount, err = m.Int64Histogram(metricPrefix + lsh.MetricExactCompareCount)
	if err != nil {
		return nil, err
	}

	inst.bucketRepsReturned, err = m.Int64Histogram(metricPrefix + lsh.MetricBucketRepsReturned)
	if err != nil {
		return nil, err
	}

	return &inst, nil
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
	start := time.Now()
	met := s.getMetrics()

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
			s.recordUpsert(ctx, met, start, lsh.ResultL1Hit, group)

			return bid, nil
		}
	}

	if resolved, ok := s.resolvedCache.Get(bid); ok {
		s.recordUpsert(ctx, met, start, lsh.ResultL2Hit, group)

		return resolved, nil
	}

	if resolved, err := s.repo.GetValue("res:" + bid); err != nil {
		return "", err
	} else if resolved != "" {
		s.resolvedCache.Add(bid, resolved)
		s.recordUpsert(ctx, met, start, lsh.ResultL3Hit, group)

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

	s.recordCandidateStats(ctx, met, bucketKeys, allReps, ids)

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

				s.recordExactChecks(ctx, met, exactChecks)
				s.recordUpsert(ctx, met, start, lsh.ResultMatch, group)

				return p.ID, nil
			}
		}

		s.recordExactChecks(ctx, met, exactChecks)
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

	s.recordUpsert(ctx, met, start, lsh.ResultNew, group)

	if met != nil {
		met.newIDTotal.Add(ctx, 1, metric.WithAttributes(lsh.AttrGroup.String(group)))
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

func (s *Service) recordUpsert(ctx context.Context, met *instruments, start time.Time, result, group string) {
	if met == nil {
		return
	}

	attrs := metric.WithAttributes(lsh.AttrResult.String(result), lsh.AttrGroup.String(group))

	met.upsertDuration.Record(ctx, time.Since(start).Seconds(), attrs)
	met.upsertTotal.Add(ctx, 1, attrs)
}

func (s *Service) recordCandidateStats(
	ctx context.Context,
	met *instruments,
	bucketKeys []string,
	allReps map[string][]repositories.Representative,
	ids []string,
) {
	if met == nil {
		return
	}

	totalReps := 0
	for _, bk := range bucketKeys {
		totalReps += len(allReps[bk])
	}

	met.bucketRepsReturned.Record(ctx, int64(totalReps))
	met.candidateCount.Record(ctx, int64(len(ids)))
}

func (s *Service) recordExactChecks(ctx context.Context, met *instruments, count int) {
	if met == nil {
		return
	}

	met.exactCompareCount.Record(ctx, int64(count))
}
