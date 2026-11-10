package lshcalc

import (
	"errors"
	"math"
)

var (
	ErrInvalidCosine               = errors.New("cosine must be between 0 and 1")
	ErrInvalidMaxMissRate          = errors.New("max miss probability must be between 0 and 1")
	ErrInvalidVectorDimensions     = errors.New("vector dimensions must be positive")
	ErrInvalidMaxBands             = errors.New("max bands must be positive")
	ErrInvalidMaxRows              = errors.New("max rows must be positive")
	ErrInvalidMinBands             = errors.New("min bands must be between 1 and max bands")
	ErrInvalidMinRows              = errors.New("min rows must be between 1 and max bands")
	ErrInvalidBackgroundSimilarity = errors.New("background similarity must be lower than cosine")
	ErrNotAchievable               = errors.New("no bands/rows tuple satisfies requested results")
)

type RequestStrict struct {
	Cosine                float64 `json:"cosine"`
	VectorDimensions      int     `json:"vector_dimensions"`
	MaxMissProbability    float64 `json:"max_miss_probability"`
	BackgroundSimilarity  float64 `json:"background_similarity,omitempty"`
	MaxBands              int     `json:"max_bands,omitempty"`
	MaxRows               int     `json:"max_rows,omitempty"`
	MinBands              int     `json:"min_bands,omitempty"`
	MinRows               int     `json:"min_rows,omitempty"`
	ReferenceRecordAmount int     `json:"reference_record_amount"`
}

func (r RequestStrict) Validate() error {
	if r.Cosine <= 0 || r.Cosine >= 1 {
		return ErrInvalidCosine
	}

	if r.MaxMissProbability <= 0 || r.MaxMissProbability >= 1 {
		return ErrInvalidMaxMissRate
	}

	if r.VectorDimensions <= 0 {
		return ErrInvalidVectorDimensions
	}

	if r.MaxBands <= 0 {
		return ErrInvalidMaxBands
	}

	if r.MaxRows <= 0 {
		return ErrInvalidMaxRows
	}

	if r.MinBands <= 0 || r.MinBands >= r.MaxBands {
		return ErrInvalidMinBands
	}

	if r.MinRows <= 0 || r.MinRows >= r.MaxRows {
		return ErrInvalidMinRows
	}

	if r.BackgroundSimilarity >= r.Cosine {
		return ErrInvalidBackgroundSimilarity
	}

	return nil
}

type ResultStrict struct {
	VectorDimensions             int     `json:"vector_dimensions"`
	Bands                        int     `json:"bands"`
	Rows                         int     `json:"rows"`
	SignatureSize                int     `json:"signature_size"`
	BucketReads                  int     `json:"bucket_reads"`
	BucketWrites                 int     `json:"bucket_writes"`
	CandidateProbability         float64 `json:"candidate_probability"`
	MissProbability              float64 `json:"miss_probability"`
	BackgroundCandidateProb      float64 `json:"background_candidate_probability"`
	EstimatedBackgroundCandidate int64   `json:"estimated_background_candidates"`
}

func CalculateStrict(req RequestStrict) (ResultStrict, error) {
	if err := req.Validate(); err != nil {
		return ResultStrict{}, err
	}

	var best ResultStrict

	hasBest := false

	for rows := req.MinRows; rows <= req.MaxRows; rows++ {
		for bands := req.MinBands; bands <= req.MaxBands; bands++ {
			candidateProbability := probability(req.Cosine, bands, rows)
			backgroundCandidateProb := probability(req.BackgroundSimilarity, bands, rows)
			missProbability := 1 - candidateProbability

			if missProbability > req.MaxMissProbability {
				continue
			}

			result := ResultStrict{
				VectorDimensions:             req.VectorDimensions,
				Bands:                        bands,
				Rows:                         rows,
				SignatureSize:                bands * rows,
				BucketReads:                  bands,
				BucketWrites:                 bands,
				CandidateProbability:         candidateProbability,
				MissProbability:              missProbability,
				BackgroundCandidateProb:      backgroundCandidateProb,
				EstimatedBackgroundCandidate: int64(math.Round(float64(req.ReferenceRecordAmount) * backgroundCandidateProb)),
			}

			if !hasBest || betterStrict(result, best) {
				best = result
				hasBest = true
			}
		}
	}

	if !hasBest {
		return ResultStrict{}, ErrNotAchievable
	}

	return best, nil
}

// betterStrict reports if the candidate result is "better" than the current
// result. Unlike better function, it does not depend on the
// RequestStrict.ReferenceRecordAmount and prioritizes background candidate
// probability first and uses MissProbability and signature size components as
// tie-breakers in that order.
func betterStrict(candidate ResultStrict, current ResultStrict) bool {
	if candidate.BackgroundCandidateProb != current.BackgroundCandidateProb {
		return candidate.BackgroundCandidateProb < current.BackgroundCandidateProb
	}

	if candidate.MissProbability != current.MissProbability {
		return candidate.MissProbability < current.MissProbability
	}

	if candidate.SignatureSize != current.SignatureSize {
		return candidate.SignatureSize < current.SignatureSize
	}

	if candidate.BucketReads != current.BucketReads {
		return candidate.BucketReads < current.BucketReads
	}

	return candidate.Rows > current.Rows
}
