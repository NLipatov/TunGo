package chacha20

const (
	signatureLength      = 64
	nonceLength          = 32
	curvePublicKeyLength = 32
	maxIpLength          = 39

	// MaxClientHelloSizeBytes consist of: 39(max ip) + 2(length headers) + 32(ed25519 pub key) + 32(curve pub key) + 32(nonce)
	MaxClientHelloSizeBytes = maxIpLength + 2 + curvePublicKeyLength + curvePublicKeyLength + nonceLength
	// minClientHelloSizeBytes consist of: 4(min ip) + 2(length headers) + 32(ed25519 pub key) + 32(curve pub key) + 32(nonce)
	minClientHelloSizeBytes = 4 + 2 + curvePublicKeyLength + curvePublicKeyLength + nonceLength
)
