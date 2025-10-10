package connection

type HandshakeFactory interface {
	NewHandshake() Handshake
}
