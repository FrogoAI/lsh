package vector

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"go.opentelemetry.io/otel/metric"

	"github.com/FrogoAI/lsh/v2"
	"github.com/FrogoAI/lsh/v2/repositories"
	"github.com/FrogoAI/multiproc/worker"
)

const cosineMargin = 0.1

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
		hasher:        NewHasher(config.Bands, config.Rows, config.VectorDimensions, config.Seed),
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
	s.metrics = nil // reset, will be re-initialized on next Upsert
}

const metricPrefix = "lsh.vector."

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
		slog.Warn("failed to create LSH vector metrics", slog.Any("error", err))

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
	start := time.Now()
	met := s.getMetrics()

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

	ids := s.collectCandidates(bucketKeys, allReps)

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

			estCosine := EstimateCosine(sig, p.Signature)

			if estCosine < (s.config.CosineThreshold - cosineMargin) {
				continue
			}

			exactChecks++

			score := ExactCosine(vector, p.Vector)
			if score >= s.config.CosineThreshold {
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
			Vector:    vector,
			GroupID:   group,
			Signature: sigCopy,
		}

		return s.repo.SaveRecord(bid, rec.toBins())
	})

	pool.Execute(func(_ context.Context) error {
		return s.repo.BatchSetRepresentative(bucketKeys, bid, 0)
	})

	err = pool.Wait()

	s.recordUpsert(ctx, met, start, lsh.ResultNew, group)

	if met != nil {
		met.newIDTotal.Add(ctx, 1, metric.WithAttributes(lsh.AttrGroup.String(group)))
	}

	return bid, err
}

// collectCandidates scans all bands, counts band overlap per candidate,
// and returns the top MaxTotalCandidates sorted by overlap count (descending).
func (s *Service) collectCandidates(
	bucketKeys []string,
	allReps map[string][]repositories.Representative,
) []string {
	bandCount := make(map[string]int)

	for _, bk := range bucketKeys {
		for _, rep := range allReps[bk] {
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
