package chacha20

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"strings"
)

func KeyToPEM(key []byte) string {
	pemString := fmt.Sprintf("-----BEGIN CHACHA20-KEY-----\n%s\n-----END CHACHA20-KEY-----", base64.StdEncoding.EncodeToString(key))
	return pemString
}

func PEMToKey(pemString string) ([]byte, error) {
	pemString = strings.TrimSpace(pemString)
	if !strings.HasPrefix(pemString, "-----BEGIN CHACHA20-KEY-----") || !strings.HasSuffix(pemString, "-----END CHACHA20-KEY-----") {
		return nil, errors.New("invalid PEM format")
	}

	encodedKey := pemString[len("-----BEGIN CHACHA20-KEY-----") : len(pemString)-len("-----END CHACHA20-KEY-----")]
	encodedKey = strings.TrimSpace(encodedKey)

	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func GenerateKey() ([]byte, error) {
	key := make([]byte, chacha20poly1305.KeySize) // ChaCha20 key size is 32 bytes (256 bits)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func Encrypt(plaintext, key []byte) (ciphertext, nonce []byte, err error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, chacha20poly1305.NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = aead.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func Decrypt(ciphertext, key, nonce []byte) (plaintext []byte, err error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}

	plaintext, err = aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
