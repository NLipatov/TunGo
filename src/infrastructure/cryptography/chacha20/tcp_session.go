package chacha20

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
)

// epochPrefixSize is the number of bytes prepended to every TCP ciphertext
// frame on the wire to identify the encryption epoch. This allows the receiver
// to route the frame to the correct session (current or previous) during a
// rekey transition without trial decryption.
const epochPrefixSize = 2

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

// TcpCrypto wraps DefaultTcpSession with dual-epoch support for seamless rekey.
//
// Every encrypted frame is prefixed with a 2-byte epoch tag on the wire:
//
//	[2B epoch BE] [ciphertext + poly1305 tag]
//
// On decrypt the epoch selects the correct session (current or previous).
// On encrypt the send-epoch session is used and the epoch is prepended.
//
// During a rekey transition both old and new sessions coexist. The previous
// session is automatically cleaned up on the first successful decrypt with
// the current epoch — TCP ordering guarantees no more old-epoch frames after that.
type TcpCrypto struct {
	mu           sync.RWMutex
	current      *DefaultTcpSession
	currentEpoch uint16
	prev         *DefaultTcpSession // nil when no rekey in progress
	prevEpoch    uint16
	sendEpoch    uint16 // which epoch to use for Encrypt
	sessionId    [32]byte
	isServer     bool
	epochCounter uint16
}

func NewTcpCrypto(id [32]byte, sendCipher, recvCipher cipher.AEAD, isServer bool) *TcpCrypto {
	sess := &DefaultTcpSession{
		SessionId:          id,
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          NewNonce(0),
		SendNonce:          NewNonce(0),
		isServer:           isServer,
		encryptionNonceBuf: [12]byte{},
		decryptionNonceBuf: [12]byte{},
	}
	return &TcpCrypto{
		current:      sess,
		currentEpoch: 0,
		sendEpoch:    0,
		sessionId:    id,
		isServer:     isServer,
		epochCounter: 0,
	}
}

func (c *TcpCrypto) Encrypt(plaintext []byte) ([]byte, error) {
	needed := len(plaintext) + chacha20poly1305.Overhead + epochPrefixSize
	if cap(plaintext) < needed {
		return nil, fmt.Errorf("insufficient capacity for epoch-prefixed encryption: cap=%d, need>=%d",
			cap(plaintext), needed)
	}

	c.mu.RLock()
	sess := c.sendSession()
	epoch := c.sendEpoch
	c.mu.RUnlock()

	ct, err := sess.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}

	// Extend buffer by 2 for epoch prefix and shift ciphertext right.
	result := ct[:len(ct)+epochPrefixSize]
	copy(result[epochPrefixSize:], ct)
	binary.BigEndian.PutUint16(result[:epochPrefixSize], epoch)

	return result, nil
}

func (c *TcpCrypto) Decrypt(data []byte) ([]byte, error) {
	if len(data) < epochPrefixSize {
		return nil, fmt.Errorf("frame too short for epoch header")
	}
	epoch := binary.BigEndian.Uint16(data[:epochPrefixSize])

	c.mu.RLock()
	sess := c.sessionForEpoch(epoch)
	cleanupPrev := epoch == c.currentEpoch && c.prev != nil
	c.mu.RUnlock()

	if sess == nil {
		return nil, fmt.Errorf("unknown epoch %d", epoch)
	}

	pt, err := sess.Decrypt(data[epochPrefixSize:])
	if err != nil {
		return nil, err
	}

	// Auto-cleanup: first successful decrypt with current epoch means
	// TCP ordering guarantees no more prev-epoch frames will arrive.
	if cleanupPrev {
		c.mu.Lock()
		c.prev = nil
		c.mu.Unlock()
	}

	return pt, nil
}

// Rekey installs a new session with fresh keys. The previous session is kept
// for decrypting in-flight old-epoch frames. The send epoch is NOT changed —
// caller must call SetSendEpoch to switch the outbound direction.
func (c *TcpCrypto) Rekey(sendKey, recvKey []byte) (uint16, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return 0, err
	}
	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	c.epochCounter++
	newEpoch := c.epochCounter

	newSess := &DefaultTcpSession{
		SessionId:          c.sessionId,
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          NewNonce(Epoch(newEpoch)),
		SendNonce:          NewNonce(Epoch(newEpoch)),
		isServer:           c.isServer,
		encryptionNonceBuf: [12]byte{},
		decryptionNonceBuf: [12]byte{},
	}

	c.prev = c.current
	c.prevEpoch = c.currentEpoch
	c.current = newSess
	c.currentEpoch = newEpoch
	// sendEpoch is intentionally NOT updated here.
	c.mu.Unlock()

	return newEpoch, nil
}

// SetSendEpoch switches the outbound encryption to the given epoch.
func (c *TcpCrypto) SetSendEpoch(epoch uint16) {
	c.mu.Lock()
	c.sendEpoch = epoch
	c.mu.Unlock()
}

// RemoveEpoch is a no-op for TCP. Cleanup of the previous session happens
// automatically in Decrypt when the first current-epoch frame arrives (TCP
// ordering guarantee). Returns true to satisfy FSM expectations.
func (c *TcpCrypto) RemoveEpoch(_ uint16) bool { return true }

func (c *TcpCrypto) sendSession() *DefaultTcpSession {
	if c.prev != nil && c.sendEpoch == c.prevEpoch {
		return c.prev
	}
	return c.current
}

func (c *TcpCrypto) sessionForEpoch(epoch uint16) *DefaultTcpSession {
	if epoch == c.currentEpoch {
		return c.current
	}
	if c.prev != nil && epoch == c.prevEpoch {
		return c.prev
	}
	return nil
}

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
