package ChaCha20

import "fmt"

type ClientSignature struct {
	ClientSignature []byte
}

func (c *ClientSignature) Read(data []byte) (*ClientSignature, error) {
	if len(data) < 64 {
		return nil, fmt.Errorf("invalid data")
	}

	c.ClientSignature = data[:64]

	return c, nil
}

func (c *ClientSignature) Write(signature *[]byte) (*[]byte, error) {
	if len(*signature) != 64 {
		return nil, fmt.Errorf("invalid signature")
	}

	arr := make([]byte, len(*signature))
	copy(arr, *signature)

	return &arr, nil
}
