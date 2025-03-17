package settings

import (
	"encoding/json"
	"errors"
	"strings"
)

type Encryption int

const (
	ChaCha20Poly1305 Encryption = iota
)

func (e *Encryption) MarshalJSON() ([]byte, error) {
	var encryptionStr string
	switch *e {
	case ChaCha20Poly1305:
		encryptionStr = "ChaCha20Poly1305"
	default:
		return nil, errors.New("unsupported protocol")
	}
	return json.Marshal(encryptionStr)
}

func (e *Encryption) UnmarshalJSON(data []byte) error {
	var encryptionStr string
	if err := json.Unmarshal(data, &encryptionStr); err != nil {
		return err
	}
	switch strings.ToLower(encryptionStr) {
	case "chacha20poly1305":
		*e = ChaCha20Poly1305
	default:
		return errors.New("unsupported protocol")
	}
	return nil
}
