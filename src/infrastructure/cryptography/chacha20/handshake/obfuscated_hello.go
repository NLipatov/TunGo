package handshake

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"golang.org/x/crypto/chacha20poly1305"
	"tungo/application"
)

const (
	minPacketLen   = 512
	maxPacketLen   = 4096
	nonceLen       = 12
	MaxEncHelloLen = 512
)

type ObfuscatedHello struct {
	hello Hello
	key   []byte           // AEAD key
	psk   []byte           // shared PSK for offset obfuscation
	hmac  application.HMAC // implementation with reusable buf (CryptoHMAC)
}

func NewFloatingObfuscatedClientHello(
	hello Hello,
	key []byte,
	psk []byte,
	hmac application.HMAC,
) ObfuscatedHello {
	return ObfuscatedHello{
		hello: hello,
		key:   key,
		psk:   psk,
		hmac:  hmac,
	}
}

func (f *ObfuscatedHello) Nonce() []byte {
	return f.hello.Nonce()
}

func (f *ObfuscatedHello) CurvePublicKey() []byte {
	return f.hello.CurvePublicKey()
}

func (f *ObfuscatedHello) MarshalBinary() ([]byte, error) {
	plainHello, err := f.hello.MarshalBinary()
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(f.key)
	if err != nil {
		return nil, err
	}

	var nonce [nonceLen]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	cipherHello := aead.Seal(nil, nonce[:], plainHello, nil)

	// Generate marker and copy first 2 bytes (protect from buf overwrite)
	hmacInput := append(f.psk, nonce[:]...)
	markerFull, hmacErr := f.hmac.Generate(hmacInput)
	if hmacErr != nil {
		return nil, hmacErr
	}
	marker := make([]byte, 2)
	copy(marker, markerFull[:2]) // copy to prevent overwrite

	// Make prefix: [marker][nonce][cipherHello]
	prefixedCipher := make([]byte, 0, 2+nonceLen+len(cipherHello))
	prefixedCipher = append(prefixedCipher, marker...)
	prefixedCipher = append(prefixedCipher, nonce[:]...)
	prefixedCipher = append(prefixedCipher, cipherHello...)

	// Calc total packet length
	totalLen := minPacketLen + int(binary.BigEndian.Uint16(nonce[:2]))%(maxPacketLen-minPacketLen)
	if totalLen < len(prefixedCipher)+32 {
		totalLen = len(prefixedCipher) + 32
	}
	packet := make([]byte, totalLen)
	if _, err := rand.Read(packet); err != nil {
		return nil, err
	}

	offset := deterministicOffset(f.hmac, f.psk, nonce[:], totalLen-len(prefixedCipher))
	copy(packet[offset:], prefixedCipher)
	return packet, nil
}

// deterministicOffset: derive offset for handshake from PSK and nonce.
// Returns offset in range [0, maxOffset)
func deterministicOffset(hmac application.HMAC, psk, nonce []byte, maxOffset int) int {
	buf := append([]byte{}, psk...) // explicit copy
	buf = append(buf, nonce...)     // explicit copy
	sum, _ := hmac.Generate(buf)    // safe: used once
	return int(binary.BigEndian.Uint32(sum[0:4]) % uint32(maxOffset))
}

func (f *ObfuscatedHello) UnmarshalBinary(data []byte) error {
	for offset := 0; offset <= len(data)-nonceLen-2; offset++ {
		marker := make([]byte, 2)
		copy(marker, data[offset:offset+2]) // avoid referencing shared buf
		nonce := make([]byte, nonceLen)
		copy(nonce, data[offset+2:offset+2+nonceLen])

		hmacInput := append([]byte{}, f.psk...)
		hmacInput = append(hmacInput, nonce...)
		expectedMarkerFull, err := f.hmac.Generate(hmacInput)
		if err != nil {
			continue
		}
		expectedMarker := make([]byte, 2)
		copy(expectedMarker, expectedMarkerFull[:2])
		if !bytes.Equal(marker, expectedMarker) {
			continue
		}
		encData := data[offset+2+nonceLen:]
		for encLen := 32; encLen <= MaxEncHelloLen && len(encData) >= encLen; encLen++ {
			aead, err := chacha20poly1305.New(f.key)
			if err != nil {
				continue
			}
			plain, err := aead.Open(nil, nonce, encData[:encLen], nil)
			if err != nil {
				continue
			}
			if err := f.hello.UnmarshalBinary(plain); err == nil {
				return nil // Success!
			}
		}
	}
	return errors.New("could not find/parse obfuscated handshake")
}
