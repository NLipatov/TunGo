package handshake

import (
	"crypto/cipher"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
)

type Session struct {
	sendCipher    cipher.AEAD
	recvCipher    cipher.AEAD
	sendNonce     uint64
	recvNonce     uint64
	isServer      bool
	SessionId     [32]byte
	MessageNumber uint64
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

// Encrypt шифрует сообщение и увеличивает счетчик nonce
func (s *Session) Encrypt(plaintext []byte, aad []byte) ([]byte, error) {
	nonce := make([]byte, chacha20poly1305.NonceSize)
	copy(nonce, uint64ToBytes(s.sendNonce)) // Используем счетчик для nonce
	s.sendNonce++                           // Увеличиваем nonce

	ciphertext := s.sendCipher.Seal(nil, nonce, plaintext, aad)
	return ciphertext, nil
}

func (s *Session) Decrypt(ciphertext []byte, aad []byte) ([]byte, error) {
	nonce := make([]byte, chacha20poly1305.NonceSize)
	copy(nonce, uint64ToBytes(s.recvNonce)) // Используем счетчик для nonce
	s.recvNonce++                           // Увеличиваем nonce

	plaintext, err := s.recvCipher.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("ошибка расшифровки: %w", err)
	}
	return plaintext, nil
}

// CreateAAD создает AAD для шифрования
func (s *Session) CreateAAD(isServerToClient bool, messageNumber uint64) []byte {
	direction := []byte("client-to-server")
	if isServerToClient {
		direction = []byte("server-to-client")
	}

	aad := append(s.SessionId[:], direction...)
	aad = append(aad, uint64ToBytes(messageNumber)...)
	return aad
}

// uint64ToBytes преобразует uint64 в байты
func uint64ToBytes(num uint64) []byte {
	b := make([]byte, 8)
	for i := uint(0); i < 8; i++ {
		b[7-i] = byte(num >> (i * 8))
	}
	return b
}

// Функция для сравнения срезов (для отладки)
func equalSlices(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
