package curve25519

import (
	"crypto/rand"
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

func Encrypt(plaintext []byte, recipientPublicKey, senderPrivateKey [32]byte) ([]byte, error) {
	sharedSecret, err := curve25519.X25519(senderPrivateKey[:], recipientPublicKey[:])
	if err != nil {
		return nil, err
	}

	encryptedData := make([]byte, len(plaintext))
	for i := range plaintext {
		encryptedData[i] = plaintext[i] ^ sharedSecret[i%32]
	}

	return encryptedData, nil
}

func Decrypt(encryptedData []byte, recipientPrivateKey, senderPublicKey [32]byte) ([]byte, error) {
	sharedSecret, err := curve25519.X25519(recipientPrivateKey[:], senderPublicKey[:])
	if err != nil {
		return nil, err
	}

	plaintext := make([]byte, len(encryptedData))
	for i := range encryptedData {
		plaintext[i] = encryptedData[i] ^ sharedSecret[i%32]
	}

	return plaintext, nil
}
