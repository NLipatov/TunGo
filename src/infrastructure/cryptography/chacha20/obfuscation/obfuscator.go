package obfuscation

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"golang.org/x/crypto/chacha20poly1305"
	"math/big"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"unsafe"
)

// ChaCha20Obfuscator implements application.Obfuscator for byte slices.
type ChaCha20Obfuscator struct {
	key            []byte           // AEAD key
	psk            []byte           // PSK for handshake offset/padding
	hmac           application.HMAC // HMAC for marker and offset
	nonceValidator *chacha20.Sliding64

	minObfuscatedPacketLen int // min final packet len (for padding)
	maxObfuscatedPacketLen int // max final packet len (for padding)
}

// NewChaCha20Obfuscator creates a new ChaCha20-based obfuscator.
func NewChaCha20Obfuscator(
	key []byte,
	psk []byte,
	hmac application.HMAC,
	nonceValidator *chacha20.Sliding64,
	minObfuscatedPacketLen, maxObfuscatedPacketLen int,
) application.Obfuscator {
	return &ChaCha20Obfuscator{
		key:                    key,
		psk:                    psk,
		hmac:                   hmac,
		nonceValidator:         nonceValidator,
		minObfuscatedPacketLen: minObfuscatedPacketLen,
		maxObfuscatedPacketLen: maxObfuscatedPacketLen,
	}
}

// Obfuscate encrypts and pads data.
func (c *ChaCha20Obfuscator) Obfuscate(plainData []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(c.key)
	if err != nil {
		return nil, err
	}

	var nonce [chacha20poly1305.NonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	if err := c.nonceValidator.Validate(nonce); err != nil {
		return nil, err
	}

	cipher := aead.Seal(nil, nonce[:], plainData, nil)
	cipherLen := uint16(len(cipher))

	// Marker for fast search
	hmacInput := append(c.psk, nonce[:]...)
	markerFull, err := c.hmac.Generate(hmacInput)
	if err != nil {
		return nil, err
	}
	marker := markerFull[:2]

	// Format: [marker][nonce][len][cipher]
	prefix := make([]byte, 0, 2+chacha20poly1305.NonceSize+2+len(cipher))
	prefix = append(prefix, marker...)
	prefix = append(prefix, nonce[:]...)
	prefix = append(prefix, byte(cipherLen>>8), byte(cipherLen))
	prefix = append(prefix, cipher...)

	// Add random padding (and fill the whole result with random data)
	padded, err := c.addPadding(prefix)
	if err != nil {
		return nil, err
	}

	// Deterministic offset (for DPI resistance)
	maxOffset := len(padded) - len(prefix)
	offset := c.deterministicOffset(nonce[:], maxOffset)
	copy(padded[offset:], prefix)
	return padded, nil
}

// addPadding pads prefix to randomized length in [min,max] (using crypto/rand).
func (c *ChaCha20Obfuscator) addPadding(data []byte) ([]byte, error) {
	if c.minObfuscatedPacketLen > c.maxObfuscatedPacketLen {
		return nil, errors.New("minLen > maxLen")
	}
	n := len(data)
	switch {
	case n >= c.maxObfuscatedPacketLen:
		return data, nil
	case c.maxObfuscatedPacketLen == c.minObfuscatedPacketLen:
		padLen := max(c.minObfuscatedPacketLen, n)
		out := make([]byte, padLen)
		copy(out, data)
		_, _ = rand.Read(out[n:])
		return out, nil
	default:
		diff := c.maxObfuscatedPacketLen - c.minObfuscatedPacketLen + 1
		nBig, err := rand.Int(rand.Reader, big.NewInt(int64(diff)))
		if err != nil {
			return nil, err
		}
		padLen := c.minObfuscatedPacketLen + int(nBig.Int64())
		if padLen < n {
			padLen = n
		}
		out := make([]byte, padLen)
		copy(out, data)
		_, _ = rand.Read(out[n:])
		return out, nil
	}
}

// deterministicOffset returns deterministic offset in [0, maxOffset) using HMAC.
func (c *ChaCha20Obfuscator) deterministicOffset(nonce []byte, maxOffset int) int {
	if maxOffset <= 0 {
		return 0
	}
	buf := append([]byte{}, c.psk...)
	buf = append(buf, nonce...)
	sum, _ := c.hmac.Generate(buf)
	return int(binary.BigEndian.Uint32(sum[:4]) % uint32(maxOffset))
}

// Deobfuscate finds and decrypts the payload in data.
func (c *ChaCha20Obfuscator) Deobfuscate(data []byte) ([]byte, error) {
	const hdrLen = 2 + chacha20poly1305.NonceSize + 2
	for offset := 0; offset <= len(data)-hdrLen; offset++ {
		marker := data[offset : offset+2]
		nonce := data[offset+2 : offset+2+chacha20poly1305.NonceSize]
		if len(nonce) != chacha20poly1305.NonceSize {
			return nil, errors.New("invalid nonce size")
		}
		if err := c.nonceValidator.Validate(*(*[12]byte)(unsafe.Pointer(&nonce[0]))); err != nil {
			return nil, err
		}
		cipherLen := int(binary.BigEndian.Uint16(data[offset+2+chacha20poly1305.NonceSize : offset+2+chacha20poly1305.NonceSize+2]))

		hmacInput := append([]byte{}, c.psk...)
		hmacInput = append(hmacInput, nonce...)
		expectedMarkerFull, err := c.hmac.Generate(hmacInput)
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
		aead, err := chacha20poly1305.New(c.key)
		if err != nil {
			continue
		}
		plain, err := aead.Open(nil, nonce, cipher, nil)
		if err != nil {
			continue
		}
		return plain, nil
	}
	return nil, errors.New("could not find/parse obfuscated handshake")
}
