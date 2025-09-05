package serviceframe

import "errors"

var (
	ErrTooShort      = errors.New("frame too short")
	ErrBadMagic      = errors.New("invalid magic")
	ErrBadVersion    = errors.New("unsupported version")
	ErrBadKind       = errors.New("invalid kind")
	ErrBodyTooLarge  = errors.New("body too large")
	ErrBodyTruncated = errors.New("frame body truncated")
)
