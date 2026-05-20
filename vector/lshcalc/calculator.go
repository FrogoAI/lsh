package lshcalc

import (
	"errors"
	"math"
)

const (
	defaultMaxBands         = 40
	defaultMaxRows          = 10
	fractionalPercentCutoff = 0.05
	percentScale            = 100
	referenceRecordAmount   = 100_000_000
)

type Request struct {
	Cosine               float64 `json:"cosine"`
	VectorDimensions     int     `json:"vector_dimensions"`
	ErrorRate            float64 `json:"error_rate"`
	MaxBands             int     `json:"max_bands,omitempty"`
	MaxRows              int     `json:"max_rows,omitempty"`
	BackgroundSimilarity float64 `json:"background_similarity,omitempty"`
}

type Result struct {
	VectorDimensions             int     `json:"vector_dimensions"`
	Bands                        int     `json:"bands"`
	Rows                         int     `json:"rows"`
	SignatureSize                int     `json:"signature_size"`
	BucketReads                  int     `json:"bucket_reads"`
	BucketWrites                 int     `json:"bucket_writes"`
	CandidateProbability         float64 `json:"candidate_probability"`
	MissProbability              float64 `json:"miss_probability"`
	BackgroundSimilarity         float64 `json:"background_similarity"`
	BackgroundCandidateProb      float64 `json:"background_candidate_probability"`
	EstimatedBackgroundCandidate int64   `json:"estimated_background_candidates"`
	ReferenceRecordAmount        int64   `json:"reference_record_amount"`
}

func Calculate(req Request) (Result, error) {
	cosine := normalizeProbability(req.Cosine)
	if cosine <= 0 || cosine >= 1 {
		return Result{}, errors.New("cosine must be between 0 and 1 or 0 and 100")
	}

	errorRate := normalizeErrorRate(req.ErrorRate)
	if errorRate <= 0 || errorRate >= 1 {
		return Result{}, errors.New("error rate must be between 0 and 1 or 0 and 100")
	}

	if req.VectorDimensions <= 0 {
		return Result{}, errors.New("vector dimensions must be positive")
	}

	maxBands := req.MaxBands
	if maxBands == 0 {
		maxBands = defaultMaxBands
	}

	if maxBands < 0 {
		return Result{}, errors.New("max bands must be positive")
	}

	maxRows := req.MaxRows
	if maxRows == 0 {
		maxRows = defaultMaxRows
	}

	if maxRows < 0 {
		return Result{}, errors.New("max rows must be positive")
	}

	backgroundSimilarity := normalizeProbability(req.BackgroundSimilarity)
	if backgroundSimilarity < 0 || backgroundSimilarity >= 1 {
		return Result{}, errors.New("background similarity must be between 0 and 1 or 0 and 100")
	}

	if backgroundSimilarity >= cosine {
		return Result{}, errors.New("background similarity must be lower than cosine")
	}

	var best Result

	hasBest := false

	for bands := 1; bands <= maxBands; bands++ {
		for rows := 1; rows <= maxRows; rows++ {
			candidateProbability := probability(cosine, bands, rows)

			missProbability := 1 - candidateProbability
			if missProbability > errorRate {
				continue
			}

			backgroundCandidateProb := probability(backgroundSimilarity, bands, rows)
			result := Result{
				VectorDimensions:             req.VectorDimensions,
				Bands:                        bands,
				Rows:                         rows,
				SignatureSize:                bands * rows,
				BucketReads:                  bands,
				BucketWrites:                 bands,
				CandidateProbability:         candidateProbability,
				MissProbability:              missProbability,
				BackgroundSimilarity:         backgroundSimilarity,
				BackgroundCandidateProb:      backgroundCandidateProb,
				EstimatedBackgroundCandidate: int64(math.Round(float64(referenceRecordAmount) * backgroundCandidateProb)),
				ReferenceRecordAmount:        referenceRecordAmount,
			}

			if !hasBest || better(result, best) {
				best = result
				hasBest = true
			}
		}
	}

	if !hasBest {
		return Result{}, errors.New("no bands/rows tuple satisfies the requested error rate")
	}

	return best, nil
}

func probability(cosine float64, bands int, rows int) float64 {
	agreeProbability := 1 - math.Acos(cosine)/math.Pi

	return 1 - math.Pow(1-math.Pow(agreeProbability, float64(rows)), float64(bands))
}

func normalizeProbability(value float64) float64 {
	if value > 1 {
		return value / percentScale
	}

	return value
}

func normalizeErrorRate(value float64) float64 {
	if value > fractionalPercentCutoff {
		return value / percentScale
	}

	return value
}

func better(candidate Result, current Result) bool {
	if candidate.EstimatedBackgroundCandidate != current.EstimatedBackgroundCandidate {
		return candidate.EstimatedBackgroundCandidate < current.EstimatedBackgroundCandidate
	}

	if candidate.BucketReads != current.BucketReads {
		return candidate.BucketReads < current.BucketReads
	}

	if candidate.SignatureSize != current.SignatureSize {
		return candidate.SignatureSize < current.SignatureSize
	}

	if candidate.MissProbability != current.MissProbability {
		return candidate.MissProbability < current.MissProbability
	}

	return candidate.Rows > current.Rows
}
