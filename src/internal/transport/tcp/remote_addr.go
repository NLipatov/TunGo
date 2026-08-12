package tcp

import (
	"io"
	"net/netip"
)

// remoteAddrConn wraps a transport and attaches a remote address.
// This allows address-unaware adapters (e.g. readDeadlineConn) to propagate the
// client's address through the adapter chain for cookie IP binding.
type remoteAddrConn struct {
	io.ReadWriteCloser
	addr netip.AddrPort
}

func WithRemoteAddr(t io.ReadWriteCloser, addr netip.AddrPort) *remoteAddrConn {
	return &remoteAddrConn{ReadWriteCloser: t, addr: addr}
}

func (r *remoteAddrConn) RemoteAddrPort() netip.AddrPort {
	return r.addr
}

// Unwrap exposes the wrapped transport for call sites that need concrete
// capabilities (for example, extracting *net.UDPConn from decorator chains).
func (r *remoteAddrConn) Unwrap() io.ReadWriteCloser {
	return r.ReadWriteCloser
}
