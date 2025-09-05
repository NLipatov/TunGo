package ip

import "net/netip"

type HeaderBuilder interface {
	BuildIPv4Packet(src, dst netip.Addr, protocol, ttl uint8, payload []byte) ([]byte, error)
	BuildIPv6Packet(src, dst netip.Addr, nh, hl uint8, payload []byte) ([]byte, error)
}
