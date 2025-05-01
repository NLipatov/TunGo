package handshake

import "fmt"

type Signature struct {
	Signature []byte
}

func NewSignature(signature []byte) Signature {
	return Signature{signature}
}

func (c *Signature) MarshalBinary() ([]byte, error) {
	if len(c.Signature) != 64 {
		return nil, fmt.Errorf("invalid signature")
	}

	arr := make([]byte, len(c.Signature))
	copy(arr, c.Signature)

	return arr, nil
}

func (c *Signature) UnmarshalBinary(data []byte) error {
	if len(data) < 64 {
		return fmt.Errorf("invalid signature")
	}

	c.Signature = data[:64]

	return nil
}
