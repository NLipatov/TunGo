package handshake

import "fmt"

type Signature struct {
	data []byte
}

func NewSignature(signature []byte) Signature {
	return Signature{
		data: signature,
	}
}

func (c *Signature) MarshalBinary() ([]byte, error) {
	if len(c.data) != 64 {
		return nil, fmt.Errorf("invalid signature")
	}

	out := make([]byte, len(c.data))
	copy(out, c.data)

	return out, nil
}

func (c *Signature) UnmarshalBinary(data []byte) error {
	if len(data) != 64 {
		return fmt.Errorf("invalid signature")
	}

	c.data = data[:64]

	return nil
}
