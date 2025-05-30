package settings

import (
	"encoding/json"
	"errors"
)

// Encryption specifies the encryption algorithm
type Encryption int

const (
	ChaCha20Poly1305 Encryption = iota
)

func (e Encryption) MarshalJSON() ([]byte, error) {
	switch e {
	case ChaCha20Poly1305:
		return json.Marshal("ChaCha20Poly1305")
	default:
		return nil, errors.New("invalid encryption")
	}
}

func (e Encryption) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "ChaCha20Poly1305" {
		e = ChaCha20Poly1305
		return nil
	}
	return errors.New("invalid encryption")
}
