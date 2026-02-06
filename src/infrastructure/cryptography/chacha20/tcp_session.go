package chacha20

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"sync"
	"tungo/infrastructure/cryptography/mem"

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

	s := &DefaultTcpSession{
		SessionId:          id,
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          NewNonce(0),
		SendNonce:          NewNonce(0),
		isServer:           isServer,
		encryptionNonceBuf: [chacha20poly1305.NonceSize]byte{},
		decryptionNonceBuf: [chacha20poly1305.NonceSize]byte{},
	}

	// Pre-fill static AAD prefix (SessionId + direction) to avoid copying on every packet.
	// Only the 12-byte nonce needs to be updated per-packet.
	copy(s.encryptionAadBuf[:sessionIdentifierLength], id[:])
	copy(s.decryptionAadBuf[:sessionIdentifierLength], id[:])
	if isServer {
		copy(s.encryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:])
		copy(s.decryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:])
	} else {
		copy(s.encryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:])
		copy(s.decryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:])
	}

	return s, nil
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
	// Compute next nonce WITHOUT committing yet.
	// We only increment after successful decryption to prevent desync
	// when an attacker sends malformed ciphertext.
	nextNonce, err := s.RecvNonce.peek()
	if err != nil {
		return nil, err
	}

	nonceBytes := nextNonce.Encode(s.decryptionNonceBuf[:])

	aad := s.CreateAAD(!s.isServer, nonceBytes, s.decryptionAadBuf[:])
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonceBytes, ciphertext, aad)
	if err != nil {
		// Decryption failed - do NOT commit nonce to prevent desync
		return nil, err
	}

	// Decryption succeeded - now commit the nonce increment
	_ = s.RecvNonce.incrementNonce()

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

	// Pre-fill static AAD prefix (SessionId + direction).
	copy(sess.encryptionAadBuf[:sessionIdentifierLength], id[:])
	copy(sess.decryptionAadBuf[:sessionIdentifierLength], id[:])
	if isServer {
		copy(sess.encryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:])
		copy(sess.decryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:])
	} else {
		copy(sess.encryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:])
		copy(sess.decryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:])
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

// Encrypt encrypts the data portion of the buffer and prepends the epoch prefix.
//
// Buffer layout contract:
//
//	input:  [ 2B epoch reserved ][ plaintext (n bytes) ][ Overhead capacity ]
//	output: [ 2B epoch          ][ ciphertext (n + 16 bytes)                ]
//
// The first epochPrefixSize bytes are overwritten with the epoch tag.
// The plaintext at buf[epochPrefixSize:] is encrypted in-place.
// The caller must ensure len(buf) >= epochPrefixSize and
// cap(buf) >= len(buf) + chacha20poly1305.Overhead.
func (c *TcpCrypto) Encrypt(buf []byte) ([]byte, error) {
	if len(buf) < epochPrefixSize {
		return nil, fmt.Errorf("buffer too short for epoch prefix: len=%d", len(buf))
	}
	data := buf[epochPrefixSize:]
	if cap(buf) < len(buf)+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("insufficient capacity for epoch-prefixed encryption: cap=%d, need>=%d",
			cap(buf), len(buf)+chacha20poly1305.Overhead)
	}

	c.mu.RLock()
	sess := c.sendSession()
	epoch := c.sendEpoch
	c.mu.RUnlock()

	ct, err := sess.Encrypt(data)
	if err != nil {
		return nil, err
	}

	// Write epoch prefix into the reserved space (no memmove needed).
	binary.BigEndian.PutUint16(buf[:epochPrefixSize], epoch)

	return buf[:epochPrefixSize+len(ct)], nil
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
		// SECURITY (R-19): Use generic error to avoid revealing epoch state.
		// Detailed logging should happen at caller if needed.
		return nil, ErrUnknownEpoch
	}

	pt, err := sess.Decrypt(data[epochPrefixSize:])
	if err != nil {
		return nil, err
	}

	// Auto-cleanup: first successful decrypt with current epoch means
	// TCP ordering guarantees no more prev-epoch frames will arrive.
	//
	// SECURITY INVARIANT: Previous session keys MUST be zeroed before release.
	// This prevents key material from persisting in memory until GC collection.
	if cleanupPrev {
		c.mu.Lock()
		if c.prev != nil {
			c.prev.zeroize()
			c.prev = nil
		}
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

	// Pre-fill static AAD prefix (SessionId + direction).
	copy(newSess.encryptionAadBuf[:sessionIdentifierLength], c.sessionId[:])
	copy(newSess.decryptionAadBuf[:sessionIdentifierLength], c.sessionId[:])
	if c.isServer {
		copy(newSess.encryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:])
		copy(newSess.decryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:])
	} else {
		copy(newSess.encryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:])
		copy(newSess.decryptionAadBuf[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:])
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

// Zeroize overwrites all key material with zeros.
// After this call, the crypto instance is unusable.
// Implements connection.CryptoZeroizer.
func (c *TcpCrypto) Zeroize() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.current != nil {
		c.current.zeroize()
	}
	if c.prev != nil {
		c.prev.zeroize()
	}
	mem.ZeroBytes(c.sessionId[:])
}

// zeroize zeros key material in a DefaultTcpSession.
func (s *DefaultTcpSession) zeroize() {
	// cipher.AEAD doesn't expose key material, but we zero what we can
	mem.ZeroBytes(s.SessionId[:])
	mem.ZeroBytes(s.encryptionAadBuf[:])
	mem.ZeroBytes(s.decryptionAadBuf[:])
	mem.ZeroBytes(s.encryptionNonceBuf[:])
	mem.ZeroBytes(s.decryptionNonceBuf[:])
	if s.SendNonce != nil {
		s.SendNonce.Zeroize()
	}
	if s.RecvNonce != nil {
		s.RecvNonce.Zeroize()
	}
}

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
	// SessionId and direction are pre-filled in the buffer at session creation.
	// Only copy the 12-byte nonce (saves 48 bytes of copying per packet).
	_ = isServerToClient // direction already set in buffer
	copy(aad[sessionIdentifierLength+directionLength:aadLength], nonce) // 48..60
	return aad[:aadLength]
}
