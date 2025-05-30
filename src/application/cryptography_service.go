package application

type CryptographyService interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

type CryptographyServiceFactory interface {
	FromHandshake(handshake Handshake, isServer bool) (CryptographyService, error)
}
