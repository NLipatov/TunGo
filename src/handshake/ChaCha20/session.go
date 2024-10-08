package ChaCha20

import (
	"crypto/cipher"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"sync"
)

type Session struct {
	sendCipher cipher.AEAD
	recvCipher cipher.AEAD
	SendNonce  [12]byte // Used for encryption
	RecvNonce  [12]byte // Used for decryption
	isServer   bool
	SessionId  [32]byte
	nonceMutex sync.Mutex
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

func (s *Session) Encrypt(plaintext []byte, aad []byte) ([]byte, error) {
	err := incrementNonce(&s.SendNonce, &s.nonceMutex)

	if err != nil {
		return nil, err
	}

	ciphertext := s.sendCipher.Seal(nil, s.SendNonce[:], plaintext, aad)
	return ciphertext, nil
}

func (s *Session) Decrypt(ciphertext []byte, aad []byte) ([]byte, error) {
	err := incrementNonce(&s.RecvNonce, &s.nonceMutex)

	if err != nil {
		return nil, err
	}

	plaintext, err := s.recvCipher.Open(nil, s.RecvNonce[:], ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
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

func incrementNonce(b *[12]byte, l *sync.Mutex) error {
	l.Lock()
	defer l.Unlock()
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			return nil
		}
	}
	return fmt.Errorf("nonce overflow")
}
