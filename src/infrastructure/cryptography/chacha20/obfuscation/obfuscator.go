package obfuscation

import (
	"bytes"
	"crypto/rand"
	"encoding"
	"encoding/binary"
	"errors"
	"golang.org/x/crypto/chacha20poly1305"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"unsafe"
)

const (
	minPacketLen = 512
	maxPacketLen = 4096
)

type ObfuscatedMarshaler interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type ChaCha20Obfuscator[T ObfuscatedMarshaler] struct {
	plain          T
	key            []byte           // AEAD key
	psk            []byte           // shared PSK for offset obfuscation
	hmac           application.HMAC // implementation with reusable buf (CryptoHMAC)
	nonceValidator *chacha20.Sliding64
}

func NewChaCha20Obfuscator[T ObfuscatedMarshaler](
	plain T,
	key []byte,
	psk []byte,
	hmac application.HMAC,
	nonceValidator *chacha20.Sliding64,
) ChaCha20Obfuscator[T] {
	return ChaCha20Obfuscator[T]{
		plain:          plain,
		key:            key,
		psk:            psk,
		hmac:           hmac,
		nonceValidator: nonceValidator,
	}
}

func (f *ChaCha20Obfuscator[T]) Plain() T {
	return f.plain
}

func (f *ChaCha20Obfuscator[T]) MarshalObfuscatedBinary() ([]byte, error) {
	plainData, err := f.plain.MarshalBinary()
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(f.key)
	if err != nil {
		return nil, err
	}

	var nonce [chacha20poly1305.NonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}

	if nonceValidationErr := f.nonceValidator.Validate(nonce); nonceValidationErr != nil {
		return nil, nonceValidationErr
	}

	cipher := aead.Seal(nil, nonce[:], plainData, nil)

	// Write ciphertext length as uint16
	cipherLen := uint16(len(cipher))

	// Generate marker and copy first 2 bytes (protect from buf overwrite)
	hmacInput := append(f.psk, nonce[:]...)
	markerFull, hmacErr := f.hmac.Generate(hmacInput)
	if hmacErr != nil {
		return nil, hmacErr
	}
	marker := make([]byte, 2)
	copy(marker, markerFull[:2]) // copy to prevent overwrite

	// Make prefix: [marker][nonce][cipherLen][cipher]
	prefixedCipher := make([]byte, 0, 2+chacha20poly1305.NonceSize+2+len(cipher))
	prefixedCipher = append(prefixedCipher, marker...)
	prefixedCipher = append(prefixedCipher, nonce[:]...)
	prefixedCipher = append(prefixedCipher, byte(cipherLen>>8), byte(cipherLen&0xff))
	prefixedCipher = append(prefixedCipher, cipher...)

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

func (f *ChaCha20Obfuscator[T]) UnmarshalObfuscatedBinary(data []byte) error {
	// 2(marker) + N(nonce) + 2(len) — minimal data len
	const hdrLen = 2 + chacha20poly1305.NonceSize + 2
	for offset := 0; offset <= len(data)-hdrLen; offset++ {
		marker := data[offset : offset+2]
		nonce := data[offset+2 : offset+2+chacha20poly1305.NonceSize]
		if len(nonce) != chacha20poly1305.NonceSize {
			return errors.New("invalid chacha20poly1305.NonceSize")
		}
		if nonceValidationErr := f.nonceValidator.Validate(*(*[12]byte)(unsafe.Pointer(&nonce[0]))); nonceValidationErr != nil {
			return nonceValidationErr
		}
		cipherLen := int(binary.BigEndian.Uint16(data[offset+2+chacha20poly1305.NonceSize : offset+2+chacha20poly1305.NonceSize+2]))

		hmacInput := append([]byte{}, f.psk...)
		hmacInput = append(hmacInput, nonce...)
		expectedMarkerFull, err := f.hmac.Generate(hmacInput)
		if err != nil {
			continue
		}
		expectedMarker := expectedMarkerFull[:2]
		if !bytes.Equal(marker, expectedMarker) {
			continue
		}
		encData := data[offset+hdrLen:]
		if len(encData) < cipherLen {
			continue
		}
		cipher := encData[:cipherLen]
		aead, err := chacha20poly1305.New(f.key)
		if err != nil {
			continue
		}
		plain, err := aead.Open(nil, nonce, cipher, nil)
		if err != nil {
			continue
		}
		if err := f.plain.UnmarshalBinary(plain); err == nil {
			return nil // Success!
		}
	}
	return errors.New("could not find/parse obfuscated handshake")
}
