package handshake

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
)

const (
	paddingLengthHeaderBytes = 1
	hmacLength               = 32
	maxPaddingLength         = 32
)

var handshakeSecret = []byte("tungo-handshake-secret")

// Obfuscator adds random padding and authenticates handshake messages using HMAC.
type Obfuscator struct{}

func (Obfuscator) Obfuscate(data []byte) ([]byte, error) {
	padBuf := make([]byte, 1)
	if _, err := rand.Read(padBuf); err != nil {
		return nil, err
	}
	padLen := int(padBuf[0]) % (maxPaddingLength + 1)
	var padding []byte
	if padLen > 0 {
		padding = make([]byte, padLen)
		if _, err := rand.Read(padding); err != nil {
			return nil, err
		}
	}
	payload := append(data, padding...)
	payload = append(payload, byte(padLen))
	hm := hmac.New(sha256.New, handshakeSecret)
	hm.Write(payload)
	payload = append(payload, hm.Sum(nil)...)
	return payload, nil
}

// Deobfuscate removes padding and verifies the message MAC. The returned boolean
// indicates whether the data was actually obfuscated.
func (Obfuscator) Deobfuscate(data []byte) ([]byte, bool, error) {
	if len(data) <= hmacLength+paddingLengthHeaderBytes {
		return data, false, nil
	}
	macStart := len(data) - hmacLength
	payload := data[:macStart]
	macBytes := data[macStart:]
	padLen := int(payload[len(payload)-1])
	if padLen > maxPaddingLength {
		return data, false, nil
	}
	padStart := len(payload) - 1 - padLen
	if padStart < 0 {
		return data, false, nil
	}
	hm := hmac.New(sha256.New, handshakeSecret)
	hm.Write(payload)
	if !hmac.Equal(hm.Sum(nil), macBytes) {
		return data, false, nil
	}
	return payload[:padStart], true, nil
}
