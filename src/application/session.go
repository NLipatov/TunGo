package application

import (
	"net/netip"
)

type Session interface {
	// ExternalAddrPort returns the external (outside VPN) address of the client.
	// Multiple clients may share the same external IP address (e.g., behind NAT).
	ExternalAddrPort() netip.AddrPort

	// InternalAddr returns the internal (inside VPN) IP address of the client.
	// Each client has a unique internal address in the virtual private network.
	InternalAddr() netip.Addr

	// ConnectionAdapter return ConnectionAdapter used to IO operations between client and server.
	ConnectionAdapter() ConnectionAdapter

	// CryptographyService returns CryptographyService used to crypto operations between client and server.
	CryptographyService() CryptographyService
}
