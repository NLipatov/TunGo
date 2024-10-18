package ChaCha20

import (
	"crypto/cipher"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"sync/atomic"
)

type Session struct {
	sendCipher    cipher.AEAD
	recvCipher    cipher.AEAD
	SendNonceLow  uint64 // Lower 8 bytes of the nonce for encryption
	SendNonceHigh uint32 // Upper 4 bytes of the nonce for encryption
	RecvNonceLow  uint64 // Lower 8 bytes of the nonce for decryption
	RecvNonceHigh uint32 // Upper 4 bytes of the nonce for decryption
	isServer      bool
	SessionId     [32]byte
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
		sendCipher:    sendCipher,
		recvCipher:    recvCipher,
		SendNonceLow:  0,
		SendNonceHigh: 0,
		RecvNonceLow:  0,
		RecvNonceHigh: 0,
		isServer:      isServer,
	}, nil
}

func (s *Session) incrementNonce(low *uint64, high *uint32) ([]byte, error) {
	// Ensure nonce does not overflow
	if atomic.LoadUint32(high) == ^uint32(0) && atomic.LoadUint64(low) == ^uint64(0) {
		return nil, fmt.Errorf("nonce overflow: maximum number of messages reached")
	}

	nonce := make([]byte, 12)

	if atomic.LoadUint64(low) == ^uint64(0) {
		atomic.AddUint32(high, 1)
		atomic.StoreUint64(low, 0)
	} else {
		atomic.AddUint64(low, 1)
	}

	lowVal := atomic.LoadUint64(low)
	highVal := atomic.LoadUint32(high)

	for i := 0; i < 8; i++ {
		nonce[i] = byte(lowVal >> (8 * i))
	}
	for i := 0; i < 4; i++ {
		nonce[8+i] = byte(highVal >> (8 * i))
	}

	return nonce, nil
}

func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	nonce, err := s.incrementNonce(&s.SendNonceLow, &s.SendNonceHigh)
	if err != nil {
		return nil, err
	}
	aad := s.CreateAAD(s.isServer, nonce)
	ciphertext := s.sendCipher.Seal(plaintext[:0], nonce, plaintext, aad)

	return ciphertext, nil
}

func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
	nonce, err := s.incrementNonce(&s.RecvNonceLow, &s.RecvNonceHigh)
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
