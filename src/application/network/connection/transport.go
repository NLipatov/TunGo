package connection

import "net/netip"

// Transport provides a single and trivial API for any supported transports
type Transport interface {
	Write([]byte) (int, error)
	Read([]byte) (int, error)
	Close() error
}

// TransportWithRemoteAddr is an optional interface for transports that can
// provide the remote peer's address. Used for cookie IP binding in DoS protection.
type TransportWithRemoteAddr interface {
	RemoteAddrPort() netip.AddrPort
}
