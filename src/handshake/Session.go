package handshake

import (
	"crypto/cipher"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
)

type Session struct {
	sendCipher cipher.AEAD
	recvCipher cipher.AEAD
	sendNonce  uint64
	recvNonce  uint64
	isServer   bool
	SessionId  [32]byte
	S2CCounter uint64 // Server to Client message counter
	C2SCounter uint64 // Client to Server message counter
}

func NewSession(sendKey, recvKey []byte, isServer bool) (*Session, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}

	return &Session{
		sendCipher: sendCipher,
		recvCipher: recvCipher,
		sendNonce:  0,
		recvNonce:  0,
		isServer:   isServer,
	}, nil
}

func (s *Session) Encrypt(plaintext []byte, aad []byte) ([]byte, error) {
	nonce := make([]byte, chacha20poly1305.NonceSize)
	copy(nonce, uint64ToBytes(s.sendNonce)) // sendNonce used as out-coming nonce counter
	s.sendNonce++

	ciphertext := s.sendCipher.Seal(nil, nonce, plaintext, aad)
	return ciphertext, nil
}

func (s *Session) Decrypt(ciphertext []byte, aad []byte) ([]byte, error) {
	nonce := make([]byte, chacha20poly1305.NonceSize)
	copy(nonce, uint64ToBytes(s.recvNonce)) // recvNonce used as in-coming nonce counter
	s.recvNonce++

	plaintext, err := s.recvCipher.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	return plaintext, nil
}

func (s *Session) CreateAAD(isServerToClient bool, messageNumber uint64) []byte {
	direction := []byte("client-to-server")
	if isServerToClient {
		direction = []byte("server-to-client")
	}

	aad := append(s.SessionId[:], direction...)
	aad = append(aad, uint64ToBytes(messageNumber)...)
	return aad
}

func uint64ToBytes(num uint64) []byte {
	b := make([]byte, 8)
	for i := uint(0); i < 8; i++ {
		b[7-i] = byte(num >> (i * 8))
	}
	return b
}
