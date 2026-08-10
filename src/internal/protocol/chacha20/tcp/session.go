package tcp

import (
	"crypto/cipher"
	"fmt"
	"tungo/internal/protocol/chacha20/internal/core"
	"tungo/internal/protocol/securemem"

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
	sendCipher         cipher.AEAD
	recvCipher         cipher.AEAD
	SendNonce          *core.Nonce
	RecvNonce          *core.Nonce
	isServer           bool
	SessionId          [32]byte
	encryptionAadBuf   [aadLength]byte
	decryptionAadBuf   [aadLength]byte
	encryptionNonceBuf [chacha20poly1305.NonceSize]byte
	decryptionNonceBuf [chacha20poly1305.NonceSize]byte
}

func NewSession(id [32]byte, sendKey, recvKey []byte, isServer bool) (*Session, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}

	return newSessionWithCiphers(id, sendCipher, recvCipher, isServer, 0), nil
}

func newSessionWithCiphers(
	id [32]byte,
	sendCipher, recvCipher cipher.AEAD,
	isServer bool,
	epoch uint16,
) *Session {
	s := &Session{
		SessionId:          id,
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          core.NewNonce(epoch),
		SendNonce:          core.NewNonce(epoch),
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

	return s
}

func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	// guarantee inplace encryption
	if cap(plaintext) < len(plaintext)+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("insufficient capacity for in-place encryption: len=%d, cap=%d, need>=%d",
			len(plaintext), cap(plaintext), len(plaintext)+chacha20poly1305.Overhead)
	}

	err := s.SendNonce.Increment()
	if err != nil {
		return nil, err
	}

	nonceBytes := s.SendNonce.Encode(s.encryptionNonceBuf[:])

	aad := s.CreateAAD(s.isServer, nonceBytes, s.encryptionAadBuf[:])
	ciphertext := s.sendCipher.Seal(plaintext[:0], nonceBytes, plaintext, aad)

	return ciphertext, nil
}

func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
	// Compute next nonce WITHOUT committing yet.
	// We only increment after successful decryption to prevent desync
	// when an attacker sends malformed ciphertext.
	// peekEncode encodes directly into the buffer — zero allocation.
	nonceBytes, err := s.RecvNonce.PeekEncode(s.decryptionNonceBuf[:])
	if err != nil {
		return nil, err
	}

	aad := s.CreateAAD(!s.isServer, nonceBytes, s.decryptionAadBuf[:])
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonceBytes, ciphertext, aad)
	if err != nil {
		// Decryption failed - do NOT commit nonce to prevent desync
		return nil, err
	}

	// Decryption succeeded - now commit the nonce increment
	_ = s.RecvNonce.Increment()

	return plaintext, nil
}

// zeroize zeros key material in a Session.
func (s *Session) zeroize() {
	// cipher.AEAD doesn't expose key material, but we zero what we can
	securemem.ZeroBytes(s.SessionId[:])
	securemem.ZeroBytes(s.encryptionAadBuf[:])
	securemem.ZeroBytes(s.decryptionAadBuf[:])
	securemem.ZeroBytes(s.encryptionNonceBuf[:])
	securemem.ZeroBytes(s.decryptionNonceBuf[:])
	if s.SendNonce != nil {
		s.SendNonce.Zeroize()
	}
	if s.RecvNonce != nil {
		s.RecvNonce.Zeroize()
	}
}

func (s *Session) CreateAAD(isServerToClient bool, nonce, aad []byte) []byte {
	// SessionId and direction are pre-filled in the buffer at session creation.
	// Only copy the 12-byte nonce (saves 48 bytes of copying per packet).
	_ = isServerToClient                                                // direction already set in buffer
	copy(aad[sessionIdentifierLength+directionLength:aadLength], nonce) // 48..60
	return aad[:aadLength]
}
