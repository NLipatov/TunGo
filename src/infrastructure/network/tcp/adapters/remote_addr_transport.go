package adapters

import (
	"net/netip"
	"tungo/application/network/connection"
)

// RemoteAddrTransport wraps a Transport and attaches a remote address,
// implementing connection.TransportWithRemoteAddr. This allows address-
// unaware adapters (e.g. ReadDeadlineTransport) to propagate the
// client's address through the adapter chain for cookie IP binding.
type RemoteAddrTransport struct {
	connection.Transport
	addr netip.AddrPort
}

func NewRemoteAddrTransport(t connection.Transport, addr netip.AddrPort) *RemoteAddrTransport {
	return &RemoteAddrTransport{Transport: t, addr: addr}
}

func (r *RemoteAddrTransport) RemoteAddrPort() netip.AddrPort {
	return r.addr
}

// Unwrap exposes the wrapped transport for call sites that need concrete
// capabilities (for example, extracting *net.UDPConn from decorator chains).
func (r *RemoteAddrTransport) Unwrap() connection.Transport {
	return r.Transport
}
