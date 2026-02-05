package connection

import (
	"net"
	"net/netip"
	"tungo/infrastructure/settings"
)

type Handshake interface {
	Id() [32]byte
	KeyClientToServer() []byte
	KeyServerToClient() []byte
	ServerSideHandshake(transport Transport) (net.IP, error)
	ClientSideHandshake(transport Transport, settings settings.Settings) error
}

// HandshakeResult contains extended handshake result for IK pattern.
// Optional interface - only IK handshake implements this.
type HandshakeResult interface {
	// ClientPubKey returns the client's X25519 static public key.
	ClientPubKey() []byte

	// AllowedIPs returns the additional prefixes this client may use as source IP.
	AllowedIPs() []netip.Prefix
}

// HandshakeWithResult is a Handshake that provides extended result info.
type HandshakeWithResult interface {
	Handshake

	// Result returns the extended handshake result, or nil if handshake not complete.
	Result() HandshakeResult
}
