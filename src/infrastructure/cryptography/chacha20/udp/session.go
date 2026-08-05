package udp

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"tungo/infrastructure/cryptography/chacha20/internal/core"
	"tungo/infrastructure/cryptography/mem"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	sessionIdentifierLength = core.SessionIdentifierLength
	directionLength         = core.DirectionLength
	aadLength               = core.AADLength
)

var (
	dirC2S = core.DirectionClientToServer
	dirS2C = core.DirectionServerToClient
)

type Session struct {
	SessionId        [32]byte
	sendCipher       cipher.AEAD
	recvCipher       cipher.AEAD
	nonce            *core.Nonce
	isServer         bool
	nonceValidator   *SlidingWindow
	epoch            uint16
	encryptionAadBuf [aadLength]byte
	decryptionAadBuf [aadLength]byte
}

func NewSession(id [32]byte, sendKey, recvKey []byte, isServer bool, epoch uint16) (*Session, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}

	return newSessionWithCiphers(id, sendCipher, recvCipher, isServer, epoch), nil
}

func newSessionWithCiphers(id [32]byte, sendCipher, recvCipher cipher.AEAD, isServer bool, epoch uint16) *Session {
	s := &Session{
		SessionId:      id,
		sendCipher:     sendCipher,
		recvCipher:     recvCipher,
		nonce:          core.NewNonce(epoch),
		isServer:       isServer,
		epoch:          epoch,
		nonceValidator: NewSlidingWindow(),
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

	return s
}

func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	// guarantee inplace encryption
	if cap(plaintext) < len(plaintext)+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("insufficient capacity for in-place encryption: len=%d, cap=%d",
			len(plaintext), cap(plaintext))
	}

	// buf: [12B nonce space | payload ...]
	if len(plaintext) < chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("encrypt: buffer too short: %d", len(plaintext))
	}

	if err := s.nonce.Increment(); err != nil {
		return nil, err
	}

	// 1) write nonce into the first 12 bytes
	nonce := plaintext[:chacha20poly1305.NonceSize]
	_ = s.nonce.Encode(nonce)

	// 2) build AAD = sessionId || direction || nonce
	aad := s.CreateAAD(s.isServer, nonce, s.encryptionAadBuf[:])

	// 3) plaintext is everything after the 12B header
	plain := plaintext[chacha20poly1305.NonceSize:]

	// 4) in-place encrypt: ciphertext overwrites plaintext region
	//    requires caller to allocate +Overhead capacity
	ct := s.sendCipher.Seal(plain[:0], nonce, plain, aad)

	// 5) return header + ciphertext view
	return plaintext[:chacha20poly1305.NonceSize+len(ct)], nil
}

func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < chacha20poly1305.NonceSize+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("cipher too short: %d", len(ciphertext))
	}
	nonceBytes := ciphertext[:chacha20poly1305.NonceSize]
	payloadBytes := ciphertext[chacha20poly1305.NonceSize:]

	// Parse nonce fields once — avoids redundant decoding in Check and Accept.
	low := binary.BigEndian.Uint64(nonceBytes[0:8])
	high := binary.BigEndian.Uint16(nonceBytes[8:10])

	// 1) check nonce (tentative - don't commit yet)
	if err := s.nonceValidator.Check(low, high); err != nil {
		return nil, err
	}

	// 2) decrypt
	aad := s.CreateAAD(!s.isServer, nonceBytes, s.decryptionAadBuf[:])
	pt, err := s.recvCipher.Open(payloadBytes[:0], nonceBytes, payloadBytes, aad)
	if err != nil {
		// Decryption failed - do NOT commit nonce to window
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	// 3) commit nonce after successful decryption
	s.nonceValidator.Accept(low, high)

	return pt, nil
}

func (s *Session) Epoch() uint16 {
	return s.epoch
}

func (s *Session) CreateAAD(isServerToClient bool, nonce, aad []byte) []byte {
	// SessionId and direction are pre-filled in the buffer at session creation.
	// Only copy the 12-byte nonce (saves 48 bytes of copying per packet).
	_ = isServerToClient                                                // direction already set in buffer
	copy(aad[sessionIdentifierLength+directionLength:aadLength], nonce) // 48..60
	return aad[:aadLength]
}

// Zeroize zeros key material in the session.
// cipher.AEAD doesn't expose key material, but we zero what we can.
//
// SECURITY INVARIANT: All session state including replay window is zeroed.
// This reduces forensic exposure of key material and packet patterns.
func (s *Session) Zeroize() {
	mem.ZeroBytes(s.SessionId[:])
	mem.ZeroBytes(s.encryptionAadBuf[:])
	mem.ZeroBytes(s.decryptionAadBuf[:])
	if s.nonce != nil {
		s.nonce.Zeroize()
	}
	// Zero replay window state (nonce history is security-sensitive)
	if s.nonceValidator != nil {
		s.nonceValidator.Zeroize()
	}
}
