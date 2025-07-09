package application

type HMAC interface {
	// Generate is used to generate(calculate) hmac
	Generate(data []byte) ([]byte, error)
	// Verify is used to verify HMAC
	Verify(data, signature []byte) error
}
