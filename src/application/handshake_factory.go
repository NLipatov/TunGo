package application

type HandshakeFactory interface {
	NewHandshake() Handshake
}
