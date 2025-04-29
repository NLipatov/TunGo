package handshake

import (
	"crypto/ed25519"
	"fmt"
)

type ClientHello struct {
	IpVersion       uint8
	IpAddressLength uint8
	IpAddress       string
	EdPublicKey     ed25519.PublicKey
	CurvePublicKey  []byte
	ClientNonce     []byte
}

func (m *ClientHello) Read(data []byte) (*ClientHello, error) {
	if len(data) < minClientHelloSizeBytes || len(data) > MaxClientHelloSizeBytes {
		return nil, fmt.Errorf("invalid message length")
	}

	m.IpVersion = data[0]

	if m.IpVersion != 4 && m.IpVersion != 6 {
		return nil, fmt.Errorf("invalid IP version")
	}

	m.IpAddressLength = data[1]

	if int(m.IpAddressLength+lengthHeaderLength) > len(data) {
		return nil, fmt.Errorf("invalid IP address length")
	}

	m.IpAddress = string(data[lengthHeaderLength : lengthHeaderLength+m.IpAddressLength])

	m.EdPublicKey = data[lengthHeaderLength+m.IpAddressLength : lengthHeaderLength+m.IpAddressLength+curvePublicKeyLength]

	m.CurvePublicKey = data[lengthHeaderLength+m.IpAddressLength+curvePublicKeyLength : lengthHeaderLength+m.IpAddressLength+curvePublicKeyLength+curvePublicKeyLength]

	m.ClientNonce = data[lengthHeaderLength+m.IpAddressLength+curvePublicKeyLength+curvePublicKeyLength : lengthHeaderLength+m.IpAddressLength+curvePublicKeyLength+curvePublicKeyLength+nonceLength]

	return m, nil
}

func (m *ClientHello) Write(ipVersion uint8, ip string, EdPublicKey ed25519.PublicKey, curvePublic *[]byte, nonce *[]byte) (*[]byte, error) {
	if ipVersion != 4 && ipVersion != 6 {
		return nil, fmt.Errorf("invalid ip version")
	}
	if ipVersion == 4 && len(ip) < 7 { //min IPv4 address length is 7 characters
		return nil, fmt.Errorf("invalid ip address")
	}
	if ipVersion == 6 && len(ip) < 2 { //min IPv6 address length is 2 characters
		return nil, fmt.Errorf("invalid ip address")
	}

	arr := make([]byte, lengthHeaderLength+len(ip)+curvePublicKeyLength+curvePublicKeyLength+nonceLength)
	arr[0] = ipVersion
	arr[1] = uint8(len(ip))
	copy(arr[lengthHeaderLength:], ip)
	copy(arr[lengthHeaderLength+len(ip):], EdPublicKey)
	copy(arr[lengthHeaderLength+len(ip)+curvePublicKeyLength:], *curvePublic)
	copy(arr[lengthHeaderLength+len(ip)+curvePublicKeyLength+curvePublicKeyLength:], *nonce)

	return &arr, nil
}
