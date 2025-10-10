package connection

type CryptoFactory interface {
	FromHandshake(
		handshake Handshake,
		isServer bool,
	) (Crypto, error)
}
