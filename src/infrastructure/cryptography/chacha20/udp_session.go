package chacha20

import (
	"crypto/cipher"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

type (
	DefaultUdpSession struct {
		SessionId        [32]byte
		encoder          DefaultUDPEncoder
		sendCipher       cipher.AEAD
		recvCipher       cipher.AEAD
		SendNonce        *Nonce
		RecvNonce        *Nonce
		isServer         bool
		nonceValidator   *Sliding64
		encryptionAadBuf [60]byte //32 bytes for sessionId, 16 bytes for direction, 12 bytes for nonce. 60 bytes total.
		decryptionAadBuf [60]byte //32 bytes for sessionId, 16 bytes for direction, 12 bytes for nonce. 60 bytes total.
	}
)

func NewUdpSession(id [32]byte, sendKey, recvKey []byte, isServer bool) (*DefaultUdpSession, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}

	return &DefaultUdpSession{
		SessionId:      id,
		sendCipher:     sendCipher,
		recvCipher:     recvCipher,
		RecvNonce:      NewNonce(),
		SendNonce:      NewNonce(),
		isServer:       isServer,
		nonceValidator: NewSliding64(),
		encoder:        DefaultUDPEncoder{},
	}, nil
}

func (s *DefaultUdpSession) Encrypt(plaintext []byte) ([]byte, error) {
	// guarantee inplace encryption
	if cap(plaintext) < len(plaintext)+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("insufficient capacity for in-place encryption: len=%d, cap=%d",
			len(plaintext), cap(plaintext))
	}

	// buf: [12B nonce space | payload ...]
	if len(plaintext) < chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("encrypt: buffer too short: %d", len(plaintext))
	}

	if err := s.SendNonce.incrementNonce(); err != nil {
		return nil, err
	}

	// 1) write nonce into the first 12 bytes
	nonce := plaintext[:chacha20poly1305.NonceSize]
	_ = s.SendNonce.Encode(nonce)

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

func (s *DefaultUdpSession) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < chacha20poly1305.NonceSize+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("cipher too short: %d", len(ciphertext))
	}
	nonceBytes := ciphertext[:chacha20poly1305.NonceSize]
	payloadBytes := ciphertext[chacha20poly1305.NonceSize:]

	// 1) validate nonce
	var n12 [chacha20poly1305.NonceSize]byte
	copy(n12[:], nonceBytes)
	if err := s.nonceValidator.Validate(n12); err != nil {
		return nil, err
	}

	// 2) decrypt
	aad := s.CreateAAD(!s.isServer, nonceBytes, s.decryptionAadBuf[:])
	pt, err := s.recvCipher.Open(payloadBytes[:0], nonceBytes, payloadBytes, aad)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	return pt, nil
}

func (s *DefaultUdpSession) CreateAAD(isServerToClient bool, nonce, aad []byte) []byte {
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
