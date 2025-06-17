package network

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

type Socket struct {
	ip   string
	port string
}

func NewSocket(ip, port string) (*Socket, error) {
	socket := &Socket{
		ip:   ip,
		port: port,
	}

	err := socket.validate()
	if err != nil {
		return nil, err
	}

	return socket, nil
}

func (s *Socket) UdpAddr() (*net.UDPAddr, error) {
	serverAddr := net.JoinHostPort(s.ip, s.port)
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, err
	}

	return udpAddr, nil
}

func (s *Socket) StringAddr() string {
	return net.JoinHostPort(s.ip, s.port)
}

func (s *Socket) validate() error {
	if s.ip != "" {
		// Reject IPv6 zone specifiers, which netip.ParseAddr would accept.
		if strings.Contains(s.ip, "%") {
			return fmt.Errorf("invalid IP %q: zone specifiers are not supported", s.ip)
		}

		if _, err := netip.ParseAddr(s.ip); err != nil {
			return fmt.Errorf("invalid IP %q: %w", s.ip, err)
		}
	}

	port, err := strconv.ParseUint(s.port, 10, 16)
	if err != nil {
		return fmt.Errorf("invalid port %q: %w", s.port, err)
	}
	if port == 0 {
		return fmt.Errorf("port must be > 0")
	}

	return nil
}
