package handshake

type Hello interface {
	Nonce() []byte
	CurvePublicKey() []byte
}
