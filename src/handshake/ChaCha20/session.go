package ChaCha20

import (
	"crypto/cipher"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
)

type Session struct {
	sendCipher cipher.AEAD
	recvCipher cipher.AEAD
	SendNonce  *Nonce
	RecvNonce  *Nonce
	isServer   bool
	SessionId  [32]byte
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
		RecvNonce:  NewNonce(),
		SendNonce:  NewNonce(),
		isServer:   isServer,
	}, nil
}

func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	nonce, err := s.SendNonce.incrementNonce()
	if err != nil {
		return nil, err
	}
	aad := s.CreateAAD(s.isServer, nonce)
	ciphertext := s.sendCipher.Seal(plaintext[:0], nonce, plaintext, aad)

	return ciphertext, nil
}

func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
	nonce, err := s.RecvNonce.incrementNonce()
	if err != nil {
		return nil, err
	}
	aad := s.CreateAAD(!s.isServer, nonce)
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonce, ciphertext, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

func (s *Session) CreateAAD(isServerToClient bool, nonce []byte) []byte {
	direction := []byte("client-to-server")
	if isServerToClient {
		direction = []byte("server-to-client")
	}

	aad := append(s.SessionId[:], direction...)
	aad = append(aad, nonce...)
	return aad
}
