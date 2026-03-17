package vector

import "errors"

var (
	ErrEmptyVector    = errors.New("empty vector")
	ErrWrongDimension = errors.New("vector dimension does not match config")
)
