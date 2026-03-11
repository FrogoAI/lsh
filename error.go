package lsh

import "errors"

var (
	ErrSignatureTooShort    = errors.New("signature too short")
	ErrShingleResultIsEmpty = errors.New("error shingle result is empty")
	ErrEmptyInputString     = errors.New("empty input string")
)
