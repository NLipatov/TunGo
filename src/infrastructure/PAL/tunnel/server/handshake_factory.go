package server

import (
	"tungo/application/network/connection"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/noise"
)

// HandshakeFactory creates Noise IK handshakes for server-side use.
type HandshakeFactory struct {
	configuration server.Configuration
	allowedPeers  noise.AllowedPeersLookup
	cookieManager *noise.CookieManager
	loadMonitor   *noise.LoadMonitor
}

// NewHandshakeFactory creates a new HandshakeFactory with IK handshake support.
// Uses provided allowedPeers for shared runtime updates across workers.
func NewHandshakeFactory(
	configuration server.Configuration,
	allowedPeers noise.AllowedPeersLookup,
	cookieManager *noise.CookieManager,
	loadMonitor *noise.LoadMonitor,
) *HandshakeFactory {
	return &HandshakeFactory{
		configuration: configuration,
		allowedPeers:  allowedPeers,
		cookieManager: cookieManager,
		loadMonitor:   loadMonitor,
	}
}

// NewHandshake creates a new IK handshake instance.
func (h *HandshakeFactory) NewHandshake() connection.Handshake {
	return noise.NewIKHandshakeServer(
		h.configuration.X25519PublicKey,
		h.configuration.X25519PrivateKey,
		h.allowedPeers,
		h.cookieManager,
		h.loadMonitor,
	)
}
