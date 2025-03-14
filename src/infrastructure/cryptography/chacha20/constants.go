package chacha20

const (
	lengthHeaderLength   = 2
	signatureLength      = 64
	nonceLength          = 32
	curvePublicKeyLength = 32
	minIpLength          = 4
	maxIpLength          = 39

	MaxClientHelloSizeBytes = maxIpLength + lengthHeaderLength + curvePublicKeyLength + curvePublicKeyLength + nonceLength
	minClientHelloSizeBytes = minIpLength + lengthHeaderLength + curvePublicKeyLength + curvePublicKeyLength + nonceLength
)
