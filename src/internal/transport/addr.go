package transport

import (
	"fmt"
	"net"
	"net/netip"
)

func NetipAddrFrom(addr net.Addr) (netip.Addr, error) {
	var netipAddr netip.Addr
	switch addr := addr.(type) {
	case *net.TCPAddr:
		netipAddr = addr.AddrPort().Addr()
	case *net.UDPAddr:
		netipAddr = addr.AddrPort().Addr()
	default:
		return netip.Addr{}, fmt.Errorf("unsupported remote address type %T", addr)
	}
	if !netipAddr.IsValid() {
		return netip.Addr{}, fmt.Errorf("invalid remote address")
	}
	return netipAddr.Unmap(), nil
}
