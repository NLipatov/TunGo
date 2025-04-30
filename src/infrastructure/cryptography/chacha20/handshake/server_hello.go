package handshake

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidData            = errors.New("handshake: invalid data length")
	ErrInvalidSignatureLength = errors.New("handshake: invalid signature length")
	ErrInvalidNonceLength     = errors.New("handshake: invalid nonce length")
	ErrInvalidCurveKeyLength  = errors.New("handshake: invalid curve public key length")
)

type ServerHello struct {
	Signature      []byte
	Nonce          []byte
	CurvePublicKey []byte
}

// NewServerHello constructs a validated ServerHello.
func NewServerHello(signature, nonce, curvePub []byte) (ServerHello, error) {
	sh := ServerHello{Signature: signature, Nonce: nonce, CurvePublicKey: curvePub}
	if _, err := sh.MarshalBinary(); err != nil {
		return ServerHello{}, fmt.Errorf("handshake: cannot create ServerHello: %w", err)
	}
	return sh, nil
}

// MarshalBinary serializes ServerHello into a fresh buffer.
func (s *ServerHello) MarshalBinary() ([]byte, error) {
	if len(s.Signature) != signatureLength {
		return nil, ErrInvalidSignatureLength
	}
	if len(s.Nonce) != nonceLength {
		return nil, ErrInvalidNonceLength
	}
	if len(s.CurvePublicKey) != curvePublicKeyLength {
		return nil, ErrInvalidCurveKeyLength
	}

	buf := make([]byte, signatureLength+nonceLength+curvePublicKeyLength)
	copy(buf[0:], s.Signature)
	copy(buf[signatureLength:], s.Nonce)
	copy(buf[signatureLength+nonceLength:], s.CurvePublicKey)
	return buf, nil
}

// UnmarshalBinary parses data into ServerHello in-place.
func (s *ServerHello) UnmarshalBinary(data []byte) error {
	min := signatureLength + nonceLength + curvePublicKeyLength
	if len(data) < min {
		return ErrInvalidData
	}
	s.Signature = append([]byte(nil), data[0:signatureLength]...)
	s.Nonce = append([]byte(nil), data[signatureLength:signatureLength+nonceLength]...)
	start := signatureLength + nonceLength
	s.CurvePublicKey = append([]byte(nil), data[start:start+curvePublicKeyLength]...)
	return nil
}
