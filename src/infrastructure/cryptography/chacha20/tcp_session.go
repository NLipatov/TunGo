package chacha20

import (
	"crypto/cipher"
	"fmt"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
)

type DefaultTcpSession struct {
	sendCipher         cipher.AEAD
	recvCipher         cipher.AEAD
	SendNonce          *Nonce
	RecvNonce          *Nonce
	isServer           bool
	SessionId          [32]byte
	encryptionAadBuf   [aadLength]byte
	decryptionAadBuf   [aadLength]byte
	encryptionNonceBuf [chacha20poly1305.NonceSize]byte
	decryptionNonceBuf [chacha20poly1305.NonceSize]byte
}

func NewTcpCryptographyService(id [32]byte, sendKey, recvKey []byte, isServer bool) (*DefaultTcpSession, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}

	return &DefaultTcpSession{
		SessionId:          id,
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          NewNonce(0),
		SendNonce:          NewNonce(0),
		isServer:           isServer,
		encryptionNonceBuf: [chacha20poly1305.NonceSize]byte{},
		decryptionNonceBuf: [chacha20poly1305.NonceSize]byte{},
	}, nil
}

func (s *DefaultTcpSession) Encrypt(plaintext []byte) ([]byte, error) {
	// guarantee inplace encryption
	if cap(plaintext) < len(plaintext)+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("insufficient capacity for in-place encryption: len=%d, cap=%d, need>=%d",
			len(plaintext), cap(plaintext), len(plaintext)+chacha20poly1305.Overhead)
	}

	err := s.SendNonce.incrementNonce()
	if err != nil {
		return nil, err
	}

	nonceBytes := s.SendNonce.Encode(s.encryptionNonceBuf[:])

	aad := s.CreateAAD(s.isServer, nonceBytes, s.encryptionAadBuf[:])
	ciphertext := s.sendCipher.Seal(plaintext[:0], nonceBytes, plaintext, aad)

	return ciphertext, nil
}

func (s *DefaultTcpSession) Decrypt(ciphertext []byte) ([]byte, error) {
	err := s.RecvNonce.incrementNonce()
	if err != nil {
		return nil, err
	}

	nonceBytes := s.RecvNonce.Encode(s.decryptionNonceBuf[:])

	aad := s.CreateAAD(!s.isServer, nonceBytes, s.decryptionAadBuf[:])
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonceBytes, ciphertext, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, err
	}

	return plaintext, nil
}

// TcpCrypto wraps DefaultTcpSession and supports immutable rekey by swapping sessions atomically.
type TcpCrypto struct {
	mu           sync.RWMutex
	session      *DefaultTcpSession
	sessionId    [32]byte
	isServer     bool
	epochCounter uint16
}

func NewTcpCrypto(id [32]byte, sendCipher, recvCipher cipher.AEAD, isServer bool) *TcpCrypto {
	return &TcpCrypto{
		session: &DefaultTcpSession{
			SessionId:          id,
			sendCipher:         sendCipher,
			recvCipher:         recvCipher,
			RecvNonce:          NewNonce(0),
			SendNonce:          NewNonce(0),
			isServer:           isServer,
			encryptionNonceBuf: [12]byte{},
			decryptionNonceBuf: [12]byte{},
		},
		sessionId:    id,
		isServer:     isServer,
		epochCounter: 0,
	}
}

func (c *TcpCrypto) Encrypt(plaintext []byte) ([]byte, error) {
	c.mu.RLock()
	s := c.session
	c.mu.RUnlock()
	return s.Encrypt(plaintext)
}

func (c *TcpCrypto) Decrypt(ciphertext []byte) ([]byte, error) {
	c.mu.RLock()
	s := c.session
	c.mu.RUnlock()
	return s.Decrypt(ciphertext)
}

// Rekey installs a new immutable TCP session with fresh nonces.
func (c *TcpCrypto) Rekey(sendKey, recvKey []byte) (uint16, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return 0, err
	}
	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return 0, err
	}
	newSess := &DefaultTcpSession{
		SessionId:          c.sessionId,
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          NewNonce(0),
		SendNonce:          NewNonce(0),
		isServer:           c.isServer,
		encryptionNonceBuf: [12]byte{},
		decryptionNonceBuf: [12]byte{},
	}
	c.mu.Lock()
	c.session = newSess
	c.epochCounter++
	epoch := c.epochCounter
	c.mu.Unlock()
	// For uniformity with UDP Rekeyer interface, return a monotonically increasing epoch surrogate.
	return epoch, nil
}

// SetSendEpoch is a no-op for TCP (no epoch routing) to satisfy Rekeyer interface reuse.
func (c *TcpCrypto) SetSendEpoch(_ uint16) {}

// RemoveEpoch is a no-op for TCP (single session only).
func (c *TcpCrypto) RemoveEpoch(_ uint16) bool { return false }

func (s *DefaultTcpSession) CreateAAD(isServerToClient bool, nonce, aad []byte) []byte {
	// aad must have len >= aadLen (60)
	copy(aad[:sessionIdentifierLength], s.SessionId[:])
	if isServerToClient {
		copy(aad[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:]) // 32..48
	} else {
		copy(aad[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:]) // 32..48
	}
	copy(aad[sessionIdentifierLength+directionLength:aadLength], nonce) // 48..60
	return aad[:aadLength]
}
