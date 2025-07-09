package handshake

type Hello interface {
	Nonce() []byte
	CurvePublicKey() []byte
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) error
}
