package handshake

import (
	"crypto/ed25519"
	"fmt"
	"net"
)

type ClientHello struct {
	ipVersion      uint8
	ipAddress      net.IP
	edPublicKey    ed25519.PublicKey
	curvePublicKey []byte
	nonce          []byte
}

func NewClientHello(
	IpVersion uint8,
	IpAddress net.IP,
	EdPublicKey ed25519.PublicKey,
	CurvePublicKey []byte,
	ClientNonce []byte) ClientHello {
	return ClientHello{
		ipVersion:      IpVersion,
		ipAddress:      IpAddress,
		edPublicKey:    EdPublicKey,
		curvePublicKey: CurvePublicKey,
		nonce:          ClientNonce,
	}
}

func (c *ClientHello) Nonce() []byte {
	return c.nonce
}

func (c *ClientHello) CurvePublicKey() []byte {
	return c.curvePublicKey
}

func (c *ClientHello) MarshalBinary() ([]byte, error) {
	if c.ipVersion != 4 && c.ipVersion != 6 {
		return nil, fmt.Errorf("invalid ip version")
	}
	if c.ipVersion == 4 && len(c.ipAddress) < 7 { //min IPv4 address length is 7 characters
		return nil, fmt.Errorf("invalid ip address")
	}
	if c.ipVersion == 6 && len(c.ipAddress) < 2 { //min IPv6 address length is 2 characters
		return nil, fmt.Errorf("invalid ip address")
	}

	arr := make([]byte, lengthHeaderLength+len(c.ipAddress)+curvePublicKeyLength+curvePublicKeyLength+nonceLength)
	arr[0] = c.ipVersion
	arr[1] = uint8(len(c.ipAddress))
	copy(arr[lengthHeaderLength:], c.ipAddress)
	copy(arr[lengthHeaderLength+len(c.ipAddress):], c.edPublicKey)
	copy(arr[lengthHeaderLength+len(c.ipAddress)+curvePublicKeyLength:], c.curvePublicKey)
	copy(arr[lengthHeaderLength+len(c.ipAddress)+curvePublicKeyLength+curvePublicKeyLength:], c.nonce)

	return arr, nil
}

func (c *ClientHello) UnmarshalBinary(data []byte) error {
	if len(data) < minClientHelloSizeBytes || len(data) > MaxClientHelloSizeBytes {
		return fmt.Errorf("invalid message length")
	}

	c.ipVersion = data[0]

	if c.ipVersion != 4 && c.ipVersion != 6 {
		return fmt.Errorf("invalid IP version")
	}

	ipAddressLength := data[1]

	if int(ipAddressLength)+lengthHeaderLength > len(data) {
		return fmt.Errorf("invalid IP address length")
	}

	offset := lengthHeaderLength
	c.ipAddress = data[offset : offset+int(ipAddressLength)]
	offset += int(ipAddressLength)

	c.edPublicKey = data[offset : offset+curvePublicKeyLength]
	offset += curvePublicKeyLength

	c.curvePublicKey = data[offset : offset+curvePublicKeyLength]
	offset += curvePublicKeyLength

	c.nonce = data[offset : offset+nonceLength]
	offset += nonceLength

	return nil
}
