package lsh

import "sync"

// NewSignaturePool creates a sync.Pool for reusable signature slices.
func NewSignaturePool(sigSize int) *sync.Pool {
	return &sync.Pool{
		New: func() interface{} {
			slice := make([]uint64, sigSize)
			return &slice
		},
	}
}
