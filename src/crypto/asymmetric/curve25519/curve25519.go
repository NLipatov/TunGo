package curve25519

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"golang.org/x/crypto/curve25519"
	"io"
)

func GenerateCurve25519KeyPair() ([32]byte, [32]byte, error) {
	var privateKey [32]byte
	if _, err := io.ReadFull(rand.Reader, privateKey[:]); err != nil {
		return [32]byte{}, [32]byte{}, err
	}

	publicKey, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return [32]byte{}, [32]byte{}, err
	}

	var pubKey [32]byte
	copy(pubKey[:], publicKey)

	return privateKey, pubKey, nil
}

func PrivateKeyToPEM(privateKey [32]byte) string {
	privateKeyBytes := privateKey[:]
	pemBlock := &pem.Block{
		Type:  "CURVE25519 PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	return string(pem.EncodeToMemory(pemBlock))
}

func PEMToPrivateKey(pemString string) ([32]byte, error) {
	block, _ := pem.Decode([]byte(pemString))
	if block == nil || block.Type != "CURVE25519 PRIVATE KEY" {
		return [32]byte{}, errors.New("invalid PEM format for private key")
	}

	var privateKey [32]byte
	copy(privateKey[:], block.Bytes)

	return privateKey, nil
}

func PublicKeyToPEM(publicKey [32]byte) string {
	publicKeyBytes := publicKey[:]
	pemBlock := &pem.Block{
		Type:  "CURVE25519 PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	return string(pem.EncodeToMemory(pemBlock))
}

func PEMToPublicKey(pemString string) ([32]byte, error) {
	block, _ := pem.Decode([]byte(pemString))
	if block == nil || block.Type != "CURVE25519 PUBLIC KEY" {
		return [32]byte{}, errors.New("invalid PEM format for public key")
	}

	var publicKey [32]byte
	copy(publicKey[:], block.Bytes)

	return publicKey, nil
}

func Encrypt(plaintext []byte, recipientPublicKey, senderPrivateKey [32]byte) (string, error) {
	sharedSecret, err := curve25519.X25519(senderPrivateKey[:], recipientPublicKey[:])
	if err != nil {
		return "", err
	}

	ciphertext := make([]byte, len(plaintext))
	for i := range plaintext {
		ciphertext[i] = plaintext[i] ^ sharedSecret[i%32]
	}

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(ciphertext string, recipientPrivateKey, senderPublicKey [32]byte) ([]byte, error) {
	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}

	sharedSecret, err := curve25519.X25519(recipientPrivateKey[:], senderPublicKey[:])
	if err != nil {
		return nil, err
	}

	plaintext := make([]byte, len(ciphertextBytes))
	for i := range ciphertextBytes {
		plaintext[i] = ciphertextBytes[i] ^ sharedSecret[i%32]
	}

	return plaintext, nil
}
