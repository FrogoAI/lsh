package lshcalc

import (
	"math"
	"testing"
)

const probabilityTolerance = 0.000001

func TestCalculateSelectsExpectedBandsRows(t *testing.T) {
	cases := []struct {
		name      string
		jaccard   float64
		wantBands int
		wantRows  int
	}{
		{name: "50 percent", jaccard: 50, wantBands: 25, wantRows: 2},
		{name: "51 percent", jaccard: 51, wantBands: 23, wantRows: 2},
		{name: "52 percent", jaccard: 52, wantBands: 22, wantRows: 2},
		{name: "53 percent", jaccard: 53, wantBands: 21, wantRows: 2},
		{name: "54 percent", jaccard: 54, wantBands: 21, wantRows: 2},
		{name: "55 percent", jaccard: 55, wantBands: 38, wantRows: 3},
		{name: "56 percent", jaccard: 56, wantBands: 36, wantRows: 3},
		{name: "57 percent", jaccard: 57, wantBands: 34, wantRows: 3},
		{name: "58 percent", jaccard: 58, wantBands: 32, wantRows: 3},
		{name: "59 percent", jaccard: 59, wantBands: 31, wantRows: 3},
		{name: "60 percent", jaccard: 60, wantBands: 29, wantRows: 3},
		{name: "61 percent", jaccard: 61, wantBands: 27, wantRows: 3},
		{name: "62 percent", jaccard: 62, wantBands: 26, wantRows: 3},
		{name: "63 percent", jaccard: 63, wantBands: 25, wantRows: 3},
		{name: "64 percent", jaccard: 64, wantBands: 38, wantRows: 4},
		{name: "65 percent", jaccard: 65, wantBands: 36, wantRows: 4},
		{name: "66 percent", jaccard: 66, wantBands: 33, wantRows: 4},
		{name: "67 percent", jaccard: 67, wantBands: 31, wantRows: 4},
		{name: "68 percent", jaccard: 68, wantBands: 29, wantRows: 4},
		{name: "69 percent", jaccard: 69, wantBands: 27, wantRows: 4},
		{name: "70 percent", jaccard: 70, wantBands: 38, wantRows: 5},
		{name: "71 percent", jaccard: 71, wantBands: 35, wantRows: 5},
		{name: "72 percent", jaccard: 72, wantBands: 33, wantRows: 5},
		{name: "73 percent", jaccard: 73, wantBands: 30, wantRows: 5},
		{name: "74 percent", jaccard: 74, wantBands: 39, wantRows: 6},
		{name: "75 percent", jaccard: 75, wantBands: 36, wantRows: 6},
		{name: "76 percent", jaccard: 76, wantBands: 33, wantRows: 6},
		{name: "77 percent", jaccard: 77, wantBands: 40, wantRows: 7},
		{name: "78 percent", jaccard: 78, wantBands: 36, wantRows: 7},
		{name: "79 percent", jaccard: 79, wantBands: 33, wantRows: 7},
		{name: "80 percent", jaccard: 80, wantBands: 38, wantRows: 8},
		{name: "81 percent", jaccard: 81, wantBands: 34, wantRows: 8},
		{name: "82 percent", jaccard: 82, wantBands: 38, wantRows: 9},
		{name: "83 percent", jaccard: 83, wantBands: 34, wantRows: 9},
		{name: "84 percent", jaccard: 84, wantBands: 36, wantRows: 10},
		{name: "85 percent", jaccard: 85, wantBands: 32, wantRows: 10},
		{name: "86 percent", jaccard: 86, wantBands: 28, wantRows: 10},
		{name: "87 percent", jaccard: 87, wantBands: 25, wantRows: 10},
		{name: "88 percent", jaccard: 88, wantBands: 22, wantRows: 10},
		{name: "89 percent", jaccard: 89, wantBands: 19, wantRows: 10},
		{name: "90 percent", jaccard: 90, wantBands: 17, wantRows: 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Calculate(Request{
				Jaccard:     tc.jaccard,
				ShingleSize: 2,
				ErrorRate:   0.001,
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
			name: "percent jaccard and fractional percent error",
			request: Request{
				Jaccard:     70,
				ShingleSize: 3,
				ErrorRate:   0.6,
			},
			wantBands:      28,
			wantRows:       5,
			wantMaxMiss:    0.006,
			wantBackground: 0.35,
		},
		{
			name: "fractional jaccard and fractional error",
			request: Request{
				Jaccard:     0.70,
				ShingleSize: 3,
				ErrorRate:   0.006,
			},
			wantBands:      28,
			wantRows:       5,
			wantMaxMiss:    0.006,
			wantBackground: 0.35,
		},
		{
			name: "percent background similarity",
			request: Request{
				Jaccard:              80,
				ShingleSize:          2,
				ErrorRate:            0.006,
				BackgroundSimilarity: 20,
			},
			wantBands:      36,
			wantRows:       9,
			wantMaxMiss:    0.006,
			wantBackground: 0.20,
		},
		{
			name: "custom max search space",
			request: Request{
				Jaccard:     0.60,
				ShingleSize: 3,
				ErrorRate:   0.04,
				MaxBands:    40,
				MaxRows:     5,
			},
			wantBands:      40,
			wantRows:       5,
			wantMaxMiss:    0.04,
			wantBackground: 0.30,
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
				Jaccard:     0.80,
				ShingleSize: 2,
				ErrorRate:   0.006,
			},
		},
		{
			name: "explicit background",
			request: Request{
				Jaccard:              0.70,
				ShingleSize:          3,
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

			wantCandidateProbability := probability(normalizeProbability(tc.request.Jaccard), result.Bands, result.Rows)
			wantBackgroundProbability := probability(result.BackgroundSimilarity, result.Bands, result.Rows)

			requireClose(t, result.CandidateProbability, wantCandidateProbability)
			requireClose(t, result.MissProbability, 1-wantCandidateProbability)
			requireClose(t, result.BackgroundCandidateProb, wantBackgroundProbability)

			if result.SignatureSize != result.Bands*result.Rows {
				t.Fatalf("signature size = %d, want %d", result.SignatureSize, result.Bands*result.Rows)
			}

			if result.BucketReads != result.Bands {
				t.Fatalf("bucket reads = %d, want %d", result.BucketReads, result.Bands)
			}

			if result.BucketWrites != result.Bands {
				t.Fatalf("bucket writes = %d, want %d", result.BucketWrites, result.Bands)
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
			name: "zero jaccard",
			request: Request{
				Jaccard:     0,
				ShingleSize: 3,
				ErrorRate:   0.001,
			},
		},
		{
			name: "full jaccard",
			request: Request{
				Jaccard:     1,
				ShingleSize: 3,
				ErrorRate:   0.001,
			},
		},
		{
			name: "negative jaccard",
			request: Request{
				Jaccard:     -0.1,
				ShingleSize: 3,
				ErrorRate:   0.001,
			},
		},
		{
			name: "zero error rate",
			request: Request{
				Jaccard:     0.50,
				ShingleSize: 3,
				ErrorRate:   0,
			},
		},
		{
			name: "non-positive shingle size",
			request: Request{
				Jaccard:     0.50,
				ShingleSize: 0,
				ErrorRate:   0.001,
			},
		},
		{
			name: "background above target",
			request: Request{
				Jaccard:              0.50,
				ShingleSize:          3,
				ErrorRate:            0.001,
				BackgroundSimilarity: 0.50,
			},
		},
		{
			name: "negative background",
			request: Request{
				Jaccard:              0.50,
				ShingleSize:          3,
				ErrorRate:            0.001,
				BackgroundSimilarity: -0.10,
			},
		},
		{
			name: "negative max bands",
			request: Request{
				Jaccard:     0.50,
				ShingleSize: 3,
				ErrorRate:   0.001,
				MaxBands:    -1,
			},
		},
		{
			name: "negative max rows",
			request: Request{
				Jaccard:     0.50,
				ShingleSize: 3,
				ErrorRate:   0.001,
				MaxRows:     -1,
			},
		},
		{
			name: "impossible search space",
			request: Request{
				Jaccard:     0.50,
				ShingleSize: 3,
				ErrorRate:   0.000001,
				MaxBands:    2,
				MaxRows:     2,
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
