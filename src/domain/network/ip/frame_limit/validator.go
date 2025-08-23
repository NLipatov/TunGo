// Package framelimit validates payload length (bytes), not including headers.
package framelimit

import (
	"errors"
)

var (
	ErrZeroCap        = errors.New("frame cap must be > 0")
	ErrCapExceeded    = errors.New("frame cap exceeded")
	ErrNegativeLength = errors.New("negative length is not allowed")
)

type Cap int // bytes

func NewCap(n int) (Cap, error) {
	if n <= 0 {
		return 0, ErrZeroCap
	}
	return Cap(n), nil
}

func (c Cap) ValidateLen(n int) error {
	if n < 0 {
		return ErrNegativeLength
	}
	if n > int(c) {
		return ErrCapExceeded
	}
	return nil
}
