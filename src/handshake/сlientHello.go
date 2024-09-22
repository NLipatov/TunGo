package handshake

import "fmt"

type ClientHello struct {
	IpVersion       uint8
	IpAddressLength uint8
	IpAddress       string
}

func (m *ClientHello) Read(data []byte) (*ClientHello, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("invalid message")
	}

	m.IpVersion = data[0]

	if m.IpVersion != 4 && m.IpVersion != 6 {
		return nil, fmt.Errorf("invalid IP version")
	}

	m.IpAddressLength = data[1]

	if int(2+m.IpAddressLength) > len(data) {
		return nil, fmt.Errorf("invalid IP address length")
	}

	m.IpAddress = string(data[2 : 2+m.IpAddressLength])

	return m, nil
}

func (m *ClientHello) Write(ipVersion uint8, ip string) (*[]byte, error) {
	if ipVersion != 4 && ipVersion != 6 {
		return nil, fmt.Errorf("invalid ip version")
	}

	if ipVersion == 4 && len(ip) < 7 { //min IPv4 address length is 7 characters
		return nil, fmt.Errorf("invalid ip address")
	}

	if ipVersion == 6 && len(ip) < 2 { //min IPv6 address length is 2 characters
		return nil, fmt.Errorf("invalid ip address")
	}

	arr := make([]byte, 2+len(ip))
	arr[0] = ipVersion
	arr[1] = uint8(len(ip))
	copy(arr[2:], ip)

	return &arr, nil
}
