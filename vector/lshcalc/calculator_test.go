package lshcalc

import (
	"math"
	"testing"
)

const probabilityTolerance = 0.000001

func TestCalculateSelectsExpectedBandsRows(t *testing.T) {
	cases := []struct {
		name      string
		cosine    float64
		wantBands int
		wantRows  int
	}{
		{name: "10 percent", cosine: 0.10, wantBands: 21, wantRows: 2},
		{name: "50 percent", cosine: 0.50, wantBands: 32, wantRows: 4},
		{name: "70 percent", cosine: 0.70, wantBands: 37, wantRows: 6},
		{name: "80 percent", cosine: 0.80, wantBands: 40, wantRows: 8},
		{name: "90 percent", cosine: 0.90, wantBands: 29, wantRows: 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Calculate(Request{
				Cosine:           tc.cosine,
				VectorDimensions: 20,
				ErrorRate:        0.001,
			})
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}

			if result.Bands != tc.wantBands {
				t.Fatalf("bands = %d, want %d", result.Bands, tc.wantBands)
			}

			if result.Rows != tc.wantRows {
				t.Fatalf("rows = %d, want %d", result.Rows, tc.wantRows)
			}
		})
	}
}

func TestCalculateNormalizesInputs(t *testing.T) {
	cases := []struct {
		name           string
		request        Request
		wantBands      int
		wantRows       int
		wantMaxMiss    float64
		wantBackground float64
	}{
		{
			name: "percent cosine and fractional percent error",
			request: Request{
				Cosine:           70,
				VectorDimensions: 20,
				ErrorRate:        0.6,
			},
			wantBands:      37,
			wantRows:       7,
			wantMaxMiss:    0.006,
			wantBackground: 0.0,
		},
		{
			name: "fractional cosine and fractional error",
			request: Request{
				Cosine:           0.70,
				VectorDimensions: 20,
				ErrorRate:        0.006,
			},
			wantBands:      37,
			wantRows:       7,
			wantMaxMiss:    0.006,
			wantBackground: 0.0,
		},
		{
			name: "percent background similarity",
			request: Request{
				Cosine:               80,
				VectorDimensions:     30,
				ErrorRate:            0.006,
				BackgroundSimilarity: 20,
			},
			wantBands:      38,
			wantRows:       9,
			wantMaxMiss:    0.006,
			wantBackground: 0.20,
		},
		{
			name: "custom max rows",
			request: Request{
				Cosine:           0.50,
				VectorDimensions: 40,
				ErrorRate:        0.004,
				MaxRows:          5,
			},
			wantBands:      40,
			wantRows:       5,
			wantMaxMiss:    0.004,
			wantBackground: 0.0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Calculate(tc.request)
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}

			if result.Bands != tc.wantBands {
				t.Fatalf("bands = %d, want %d", result.Bands, tc.wantBands)
			}

			if result.Rows != tc.wantRows {
				t.Fatalf("rows = %d, want %d", result.Rows, tc.wantRows)
			}

			if result.MissProbability > tc.wantMaxMiss {
				t.Fatalf("miss probability = %f, want <= %f", result.MissProbability, tc.wantMaxMiss)
			}

			requireClose(t, result.BackgroundSimilarity, tc.wantBackground)
		})
	}
}

func TestCalculateResultProbabilities(t *testing.T) {
	cases := []struct {
		name    string
		request Request
	}{
		{
			name: "default background",
			request: Request{
				Cosine:           0.80,
				VectorDimensions: 20,
				ErrorRate:        0.006,
			},
		},
		{
			name: "explicit background",
			request: Request{
				Cosine:               0.70,
				VectorDimensions:     30,
				ErrorRate:            0.001,
				BackgroundSimilarity: 0.25,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Calculate(tc.request)
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}

			wantCandidateProbability := probability(normalizeProbability(tc.request.Cosine), result.Bands, result.Rows)
			wantBackgroundProbability := probability(result.BackgroundSimilarity, result.Bands, result.Rows)
			wantEstimatedBackground := int64(math.Round(float64(referenceRecordAmount) * wantBackgroundProbability))

			requireClose(t, result.CandidateProbability, wantCandidateProbability)
			requireClose(t, result.MissProbability, 1-wantCandidateProbability)
			requireClose(t, result.BackgroundCandidateProb, wantBackgroundProbability)

			if result.EstimatedBackgroundCandidate != wantEstimatedBackground {
				t.Fatalf(
					"estimated background candidate = %d, want %d",
					result.EstimatedBackgroundCandidate,
					wantEstimatedBackground,
				)
			}

			if result.SignatureSize != result.Bands*result.Rows {
				t.Fatalf("signature size = %d, want %d", result.SignatureSize, result.Bands*result.Rows)
			}

			if result.BucketReads != result.Bands {
				t.Fatalf("bucket reads = %d, want %d", result.BucketReads, result.Bands)
			}

			if result.BucketWrites != result.Bands {
				t.Fatalf("bucket writes = %d, want %d", result.BucketWrites, result.Bands)
			}

			if result.VectorDimensions != tc.request.VectorDimensions {
				t.Fatalf("vector dimensions = %d, want %d", result.VectorDimensions, tc.request.VectorDimensions)
			}

			if result.ReferenceRecordAmount != referenceRecordAmount {
				t.Fatalf("reference record amount = %d, want %d", result.ReferenceRecordAmount, referenceRecordAmount)
			}
		})
	}
}

func TestCalculateRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name    string
		request Request
	}{
		{
			name: "zero cosine",
			request: Request{
				Cosine:           0,
				VectorDimensions: 20,
				ErrorRate:        0.001,
			},
		},
		{
			name: "full cosine",
			request: Request{
				Cosine:           1,
				VectorDimensions: 20,
				ErrorRate:        0.001,
			},
		},
		{
			name: "negative cosine",
			request: Request{
				Cosine:           -0.1,
				VectorDimensions: 20,
				ErrorRate:        0.001,
			},
		},
		{
			name: "zero error rate",
			request: Request{
				Cosine:           0.50,
				VectorDimensions: 20,
				ErrorRate:        0,
			},
		},
		{
			name: "full error rate",
			request: Request{
				Cosine:           0.50,
				VectorDimensions: 20,
				ErrorRate:        100,
			},
		},
		{
			name: "non-positive vector dimensions",
			request: Request{
				Cosine:           0.50,
				VectorDimensions: 0,
				ErrorRate:        0.001,
			},
		},
		{
			name: "background equal to target",
			request: Request{
				Cosine:               0.50,
				VectorDimensions:     20,
				ErrorRate:            0.001,
				BackgroundSimilarity: 0.50,
			},
		},
		{
			name: "background above target",
			request: Request{
				Cosine:               0.50,
				VectorDimensions:     20,
				ErrorRate:            0.001,
				BackgroundSimilarity: 0.60,
			},
		},
		{
			name: "negative background",
			request: Request{
				Cosine:               0.50,
				VectorDimensions:     20,
				ErrorRate:            0.001,
				BackgroundSimilarity: -0.10,
			},
		},
		{
			name: "full background",
			request: Request{
				Cosine:               0.50,
				VectorDimensions:     20,
				ErrorRate:            0.001,
				BackgroundSimilarity: 1,
			},
		},
		{
			name: "negative max bands",
			request: Request{
				Cosine:           0.50,
				VectorDimensions: 20,
				ErrorRate:        0.001,
				MaxBands:         -1,
			},
		},
		{
			name: "negative max rows",
			request: Request{
				Cosine:           0.50,
				VectorDimensions: 20,
				ErrorRate:        0.001,
				MaxRows:          -1,
			},
		},
		{
			name: "impossible search space",
			request: Request{
				Cosine:           0.50,
				VectorDimensions: 20,
				ErrorRate:        0.000001,
				MaxBands:         2,
				MaxRows:          2,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Calculate(tc.request)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNormalizeErrorRate(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  float64
	}{
		{name: "fractional miss rate", value: 0.001, want: 0.001},
		{name: "fractional percent miss rate", value: 0.6, want: 0.006},
		{name: "one percent miss rate", value: 1, want: 0.01},
		{name: "ten percent miss rate", value: 10, want: 0.10},
		{name: "cutoff remains fractional", value: 0.05, want: 0.05},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeErrorRate(tc.value)

			requireClose(t, got, tc.want)
		})
	}
}

func requireClose(t *testing.T, got, want float64) {
	t.Helper()

	if math.Abs(got-want) > probabilityTolerance {
		t.Fatalf("got %f, want %f", got, want)
	}
}
