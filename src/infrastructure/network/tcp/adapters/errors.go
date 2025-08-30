package adapters

import "errors"

var (
	ErrInvalidLengthPrefixHeader = errors.New("invalid length prefix header")
	ErrFrameCapExceeded          = errors.New("frame exceeds maximum allowed frame size")
	ErrZeroLengthFrame           = errors.New("zero length frame")
)
