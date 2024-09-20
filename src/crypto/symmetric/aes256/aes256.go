package aes256

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

func KeyToPEM(key []byte) string {
	pemString := fmt.Sprintf("-----BEGIN AES-256 KEY-----\n%s\n-----END AES-256 KEY-----", base64.StdEncoding.EncodeToString(key))
	return pemString
}

func PEMToKey(pemString string) ([]byte, error) {
	pemString = strings.TrimSpace(pemString)
	if !strings.HasPrefix(pemString, "-----BEGIN AES-256 KEY-----") || !strings.HasSuffix(pemString, "-----END AES-256 KEY-----") {
		return nil, errors.New("invalid PEM format")
	}

	encodedKey := pemString[len("-----BEGIN AES-256 KEY-----") : len(pemString)-len("-----END AES-256 KEY-----")]
	encodedKey = strings.TrimSpace(encodedKey)

	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func GenerateKey() ([]byte, error) {
	key := make([]byte, 32) //32 bytes - 256 bits
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func Encrypt(plaintext, key []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = aesGCM.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func Decrypt(ciphertext, key, nonce []byte) (plaintext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err = aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
