package handshake

type Nonce interface {
	Nonce() []byte
	CurvePublicKey() []byte
}
