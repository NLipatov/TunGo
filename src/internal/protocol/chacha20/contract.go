package chacha20

type Crypto interface {
	Encrypt(buffer []byte) ([]byte, error)
	Decrypt(buffer []byte) ([]byte, uint16, error)
}
