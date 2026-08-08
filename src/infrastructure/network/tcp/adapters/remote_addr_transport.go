package adapters

import (
	"io"
	"net/netip"
)

// RemoteAddrTransport wraps a transport and attaches a remote address.
// This allows address-unaware adapters (e.g. ReadDeadlineTransport) to propagate the
// client's address through the adapter chain for cookie IP binding.
type RemoteAddrTransport struct {
	io.ReadWriteCloser
	addr netip.AddrPort
}

func NewRemoteAddrTransport(t io.ReadWriteCloser, addr netip.AddrPort) *RemoteAddrTransport {
	return &RemoteAddrTransport{ReadWriteCloser: t, addr: addr}
}

func (r *RemoteAddrTransport) RemoteAddrPort() netip.AddrPort {
	return r.addr
}

// Unwrap exposes the wrapped transport for call sites that need concrete
// capabilities (for example, extracting *net.UDPConn from decorator chains).
func (r *RemoteAddrTransport) Unwrap() io.ReadWriteCloser {
	return r.ReadWriteCloser
}
