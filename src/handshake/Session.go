package handshake

import (
	"crypto/cipher"
	"golang.org/x/crypto/chacha20poly1305"
)

type Session struct {
	sendCipher cipher.AEAD
	recvCipher cipher.AEAD
	sendNonce  uint64
	recvNonce  uint64
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

		sendNonce: 0,

		recvNonce: 0,

		isServer: isServer,
	}, nil

}

// Encrypt шифрует сообщение и увеличивает счетчик nonce

func (s *Session) Encrypt(plaintext []byte, aad []byte) ([]byte, error) {

	nonce := make([]byte, chacha20poly1305.NonceSize)

	// Используем счетчик для nonce

	copy(nonce, uint64ToBytes(s.sendNonce))

	s.sendNonce++

	ciphertext := s.sendCipher.Seal(nil, nonce, plaintext, aad)

	return ciphertext, nil

}

// Decrypt расшифровывает сообщение с использованием текущего nonce

func (s *Session) Decrypt(ciphertext []byte, aad []byte) ([]byte, error) {

	nonce := make([]byte, chacha20poly1305.NonceSize)

	// Используем счетчик для nonce

	copy(nonce, uint64ToBytes(s.recvNonce))

	s.recvNonce++

	plaintext, err := s.recvCipher.Open(nil, nonce, ciphertext, aad)

	return plaintext, err

}

// Функция для создания AAD

func createAAD(sessionID []byte, isServerToClient bool, messageNumber uint64) []byte {

	direction := []byte("client-to-server")

	if isServerToClient {

		direction = []byte("server-to-client")

	}

	aad := append(sessionID, direction...)

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

// Вспомогательная функция для сравнения срезов

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
