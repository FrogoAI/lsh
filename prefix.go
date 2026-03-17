package lsh

import (
	"encoding/hex"

	"github.com/cespare/xxhash/v2"

	"github.com/FrogoAI/packer"
)

// HashVersion computes a deterministic prefix from a group name and config fields.
// Any change in config fields produces a different prefix, invalidating old buckets.
func HashVersion(group string, configFields ...any) (string, error) {
	enc := packer.NewBinaryEncoder()

	err := enc.Encode(group)
	if err != nil {
		return "", err
	}

	for _, f := range configFields {
		err = enc.Encode(f)
		if err != nil {
			return "", err
		}
	}

	s := xxhash.New()
	// xxhash.Digest.Write never returns an error.
	_, _ = s.Write(enc.Bytes())

	return hex.EncodeToString(s.Sum(nil)), nil
}
