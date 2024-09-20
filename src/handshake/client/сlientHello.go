package client

import (
	"fmt"
)

type ClientHello struct {
	IpVersion       uint8
	IpAddressLength uint8
	IpAddress       string
	PublicKey       [32]byte
}

func (m *ClientHello) Read(data []byte) (*ClientHello, error) {
	if len(data) < int(2+m.IpAddressLength+32) {
		return nil, fmt.Errorf("invalid message length")
	}

	m.IpVersion = data[0]

	if m.IpVersion != 4 && m.IpVersion != 6 {
		return nil, fmt.Errorf("invalid IP version")
	}

	m.IpAddressLength = data[1]

	m.IpAddress = string(data[2 : 2+m.IpAddressLength])

	copy(m.PublicKey[:], data[2+m.IpAddressLength:32+2+m.IpAddressLength])

	return m, nil
}

func (m *ClientHello) Write(ipVersion uint8, ip string, pubKey [32]byte) ([]byte, error) {
	if ipVersion != 4 && ipVersion != 6 {
		return nil, fmt.Errorf("invalid ip version")
	}

	if ipVersion == 4 && (len(ip) < 7 || len(ip) > 15) {
		return nil, fmt.Errorf("invalid IPv4 address")
	}

	if ipVersion == 6 && (len(ip) < 2 || len(ip) > 39) {
		return nil, fmt.Errorf("invalid IPv6 address")
	}

	arr := make([]byte, 32+2+len(ip))
	arr[0] = ipVersion
	arr[1] = uint8(len(ip))
	copy(arr[2:], ip)

	copy(arr[2+len(ip):], pubKey[:])

	return arr, nil
}
