package ChaCha20

import "fmt"

type ClientSignature struct {
	ClientSignature []byte
}

func (s *ClientSignature) Read(data []byte) (*ClientSignature, error) {
	if len(data) < 64 {
		return nil, fmt.Errorf("invalid data")
	}

	s.ClientSignature = data[:64]

	return s, nil
}

func (m *ClientSignature) Write(signature *[]byte) (*[]byte, error) {
	if len(*signature) != 64 {
		return nil, fmt.Errorf("invalid signature")
	}

	arr := make([]byte, len(*signature))
	copy(arr, *signature)

	return &arr, nil
}
