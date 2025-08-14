package ip

import "fmt"

type Version byte

const (
	Unknown Version = 0
	V4      Version = 4
	V6      Version = 6
)

// Valid reports whether v is a supported IP version.
func (v Version) Valid() bool { return v == V4 || v == V6 }

// FromByte constructs Version from a single byte.
func FromByte(b byte) (Version, error) {
	switch b {
	case 4:
		return V4, nil
	case 6:
		return V6, nil
	default:
		return 0, fmt.Errorf("invalid IP version: %d", b)
	}
}

// Byte returns the numeric representation (4 or 6).
func (v Version) Byte() byte { return byte(v) }
