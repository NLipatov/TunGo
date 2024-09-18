package packages

import (
	"fmt"
	"net"
)

type IPHeader interface {
	GetDestinationIP() net.IP
}

func Parse(packet []byte) (IPHeader, error) {
	if packet[0]>>4 == 4 { //Right shift to cut off a 'IHL' part of first IPv4 packet byte
		v4Header, err := ParseIPv4Header(packet)
		if err != nil {
			return nil, err
		}
		return v4Header, nil
	}

	if packet[0]>>4 == 6 { //Right shift to cut off a 'Traffic class' part of first IPv6 packet byte
		v6Header, err := ParseIPv6Header(packet)
		if err != nil {
			return nil, err
		}
		return v6Header, nil
	}

	return nil, fmt.Errorf("unsupported packet version")
}
