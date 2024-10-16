package ChaCha20

import (
	"crypto/cipher"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"sync"
)

type Session struct {
	sendCipher     cipher.AEAD
	recvCipher     cipher.AEAD
	SendNonce      [12]byte // Used for encryption
	RecvNonce      [12]byte // Used for decryption
	isServer       bool
	SessionId      [32]byte
	sendNonceMutex sync.Mutex
	recvNonceMutex sync.Mutex
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
		SendNonce:  [12]byte{},
		RecvNonce:  [12]byte{},
		isServer:   isServer,
	}, nil
}

func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	s.sendNonceMutex.Lock()
	defer s.sendNonceMutex.Unlock()

	aad := s.CreateAAD(s.isServer, s.SendNonce)

	ciphertext := s.sendCipher.Seal(plaintext[:0], s.SendNonce[:], plaintext, aad)

	err := incrementNonce(&s.SendNonce)
	if err != nil {
		return nil, err
	}

	return ciphertext, nil
}

func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
	s.recvNonceMutex.Lock()
	defer s.recvNonceMutex.Unlock()

	aad := s.CreateAAD(!s.isServer, s.RecvNonce)

	plaintext, err := s.recvCipher.Open(ciphertext[:0], s.RecvNonce[:], ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	err = incrementNonce(&s.RecvNonce)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func (s *Session) CreateAAD(isServerToClient bool, nonce [12]byte) []byte {
	direction := []byte("client-to-server")
	if isServerToClient {
		direction = []byte("server-to-client")
	}

	aad := append(s.SessionId[:], direction...)
	aad = append(aad, nonce[:]...)
	return aad
}

func incrementNonce(b *[12]byte) error {
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			return nil
		}
	}
	return fmt.Errorf("nonce overflow")
}
