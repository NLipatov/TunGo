package server

import "tungo/infrastructure/cryptography/noise"

func (s *Server) newHandshake() *noise.IKHandshake {
	return noise.NewIKHandshakeServer(
		s.configuration.X25519PublicKey,
		s.configuration.X25519PrivateKey,
		s.allowedPeers,
		s.cookieManager,
		s.loadMonitor,
	)
}
