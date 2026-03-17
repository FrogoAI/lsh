package lsh

import (
	"strconv"

	"github.com/cespare/xxhash/v2"
)

// ComputeBands splits a signature into bands and hashes each band into a bucket key.
// This is the universal LSH banding step — works for any signature type (MinHash, hyperplane, etc.).
func ComputeBands(signature []uint64, bands, rows int) ([]string, error) {
	sigSize := bands * rows
	if len(signature) < sigSize {
		return nil, ErrSignatureTooShort
	}

	bucketKeys := make([]string, bands)

	buf := make([]byte, rows*8) //nolint:mnd
	d := xxhash.New()

	for i := 0; i < bands; i++ {
		start := i * rows

		offset := 0

		for k := 0; k < rows; k++ {
			val := signature[start+k]

			buf[offset] = byte(val)
			buf[offset+1] = byte(val >> 8)  //nolint:mnd
			buf[offset+2] = byte(val >> 16) //nolint:mnd
			buf[offset+3] = byte(val >> 24) //nolint:mnd
			buf[offset+4] = byte(val >> 32) //nolint:mnd
			buf[offset+5] = byte(val >> 40) //nolint:mnd
			buf[offset+6] = byte(val >> 48) //nolint:mnd
			buf[offset+7] = byte(val >> 56) //nolint:mnd

			offset += 8 //nolint:mnd
		}

		d.Reset()

		_, err := d.Write(buf)
		if err != nil {
			return nil, err
		}

		bucketKeys[i] = "b" + strconv.Itoa(i) + ":" + strconv.FormatUint(d.Sum64(), 16)
	}

	return bucketKeys, nil
}

// PrefixKeys prepends a config-version prefix to each bucket key.
func PrefixKeys(prefix string, keys []string) []string {
	result := make([]string, len(keys))
	for i, k := range keys {
		result[i] = prefix + ":" + k
	}

	return result
}
