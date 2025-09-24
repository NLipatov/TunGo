package adapters

import "errors"

var (
	ErrInvalidLengthPrefixHeader = errors.New("invalid length prefix header")
	ErrZeroLengthFrame           = errors.New("zero length frame")
)
