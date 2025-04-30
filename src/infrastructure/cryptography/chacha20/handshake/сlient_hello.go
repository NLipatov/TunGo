package handshake

import (
	"encoding"
	"errors"
	"fmt"
)

var (
	ErrInvalidMessageLength = errors.New("handshake: invalid message length")
	ErrInvalidIPVersion     = errors.New("handshake: invalid IP version")
	ErrInvalidIPAddrLength  = errors.New("handshake: invalid IP address length")
)

// ClientHello represents the initial handshake message from client.
type ClientHello struct {
	IpVersion     uint8
	IpAddress     string
	Ed25519PubKey []byte
	CurvePubKey   []byte
	ClientNonce   []byte
}

// Ensure compatibility with BinaryMarshaler/Unmarshaler
var _ encoding.BinaryMarshaler = (*ClientHello)(nil)
var _ encoding.BinaryUnmarshaler = (*ClientHello)(nil)

// MarshalBinary serializes ClientHello into a fresh buffer.
func (m *ClientHello) MarshalBinary() ([]byte, error) {
	if m.IpVersion != 4 && m.IpVersion != 6 {
		return nil, ErrInvalidIPVersion
	}

	ipLen := len(m.IpAddress)
	// lengthHeaderLength = 2
	if int(m.IpVersion) == 4 && ipLen < 7 || int(m.IpVersion) == 6 && ipLen < 2 {
		return nil, ErrInvalidIPAddrLength
	}
	// total length = 2 + ipLen + Ed + Curve + nonce
	buf := make([]byte, lengthHeaderLength+ipLen+curvePublicKeyLength+curvePublicKeyLength+nonceLength)
	buf[0] = m.IpVersion
	buf[1] = uint8(ipLen)
	copy(buf[lengthHeaderLength:], m.IpAddress)
	copy(buf[lengthHeaderLength+ipLen:], m.Ed25519PubKey)
	copy(buf[lengthHeaderLength+ipLen+curvePublicKeyLength:], m.CurvePubKey)
	copy(buf[lengthHeaderLength+ipLen+curvePublicKeyLength+curvePublicKeyLength:], m.ClientNonce)
	return buf, nil
}

// UnmarshalBinary parses data into ClientHello in-place.
func (m *ClientHello) UnmarshalBinary(data []byte) error {
	if len(data) < minClientHelloSizeBytes || len(data) > MaxClientHelloSizeBytes {
		return ErrInvalidMessageLength
	}
	version := data[0]
	if version != 4 && version != 6 {
		return ErrInvalidIPVersion
	}
	m.IpVersion = version
	m.IpAddress = ""
	length := int(data[1])
	if length+lengthHeaderLength > len(data) {
		return ErrInvalidIPAddrLength
	}
	m.IpAddress = string(data[lengthHeaderLength : lengthHeaderLength+length])
	start := lengthHeaderLength + length
	m.Ed25519PubKey = append([]byte(nil), data[start:start+curvePublicKeyLength]...)
	m.CurvePubKey = append([]byte(nil), data[start+curvePublicKeyLength:start+2*curvePublicKeyLength]...)
	m.ClientNonce = append([]byte(nil), data[start+2*curvePublicKeyLength:start+2*curvePublicKeyLength+nonceLength]...)
	return nil
}

// NewClientHello validates fields and returns a ClientHello
func NewClientHello(version uint8, ip string, edPub, curvePub, nonce []byte) (*ClientHello, error) {
	ch := &ClientHello{IpVersion: version, IpAddress: ip, Ed25519PubKey: edPub, CurvePubKey: curvePub, ClientNonce: nonce}
	if _, err := ch.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("handshake: cannot create ClientHello: %w", err)
	}
	return ch, nil
}
