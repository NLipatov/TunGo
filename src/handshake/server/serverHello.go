package server

import (
	"etha-tunnel/crypto/asymmetric/curve25519"
	"fmt"
)

type ServerHello struct {
	CC20Key   [32]byte
	PublicKey [32]byte
}

func (s *ServerHello) Read(data []byte, recipientPrivateKey, senderPublicKey [32]byte) (*ServerHello, error) {
	if len(data) != 32 {
		return nil, fmt.Errorf("invalid message")
	}

	cc20Key, err := curve25519.Decrypt(data[:], recipientPrivateKey[:], senderPublicKey[:])
	if err != nil {
		return nil, err
	}

	copy(s.CC20Key[:], cc20Key)

	return s, nil
}

func (s *ServerHello) Write(cc20Key []byte, recipientPublicKey, senderPrivateKey [32]byte, senderPublicKey [32]byte) ([]byte, error) {
	encryptedCC20Key, err := curve25519.Encrypt(cc20Key, recipientPublicKey, senderPrivateKey)
	if err != nil {
		return nil, err
	}

	arr := make([]byte, 64)
	copy(arr[:32], encryptedCC20Key)
	copy(arr[32:], senderPublicKey[:])

	return arr, err
}
