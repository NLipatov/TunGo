package handshake

import (
	"encoding"
	"errors"
	"fmt"
)

var (
	// ErrInvalidClientSignatureLength is returned when signature != 64 bytes
	ErrInvalidClientSignatureLength = errors.New("handshake: invalid client signature length")
)

// ClientSignature holds the 64-byte signature sent by client.
type ClientSignature struct {
	Signature []byte
}

// Ensure interface compliance for binary marshaling.
var (
	_ encoding.BinaryMarshaler   = (*ClientSignature)(nil)
	_ encoding.BinaryUnmarshaler = (*ClientSignature)(nil)
)

// MarshalBinary returns the signature as a 64-byte slice.
func (cs *ClientSignature) MarshalBinary() ([]byte, error) {
	if len(cs.Signature) != signatureLength {
		return nil, ErrInvalidClientSignatureLength
	}
	buf := make([]byte, signatureLength)
	copy(buf, cs.Signature)
	return buf, nil
}

// UnmarshalBinary sets cs.Signature from the leading 64 bytes of data.
func (cs *ClientSignature) UnmarshalBinary(data []byte) error {
	if len(data) < signatureLength {
		return ErrInvalidClientSignatureLength
	}
	cs.Signature = append([]byte(nil), data[:signatureLength]...)
	return nil
}

// NewClientSignature constructs and validates a ClientSignature.
func NewClientSignature(sig []byte) (*ClientSignature, error) {
	cs := &ClientSignature{Signature: sig}
	if _, err := cs.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("handshake: cannot create ClientSignature: %w", err)
	}
	return cs, nil
}
