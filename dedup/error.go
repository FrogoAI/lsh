package dedup

import "errors"

var (
	ErrShingleResultIsEmpty = errors.New("error shingle result is empty")
	ErrEmptyInputString     = errors.New("empty input string")
)
