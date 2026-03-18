package lsh

import "errors"

var (
	ErrSignatureTooShort = errors.New("signature too short")
	ErrInvalidConfig     = errors.New("invalid LSH config")
)
