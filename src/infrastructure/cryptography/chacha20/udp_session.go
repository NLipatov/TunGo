package chacha20

import (
	"crypto/cipher"
	"fmt"
	"unsafe"

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

func (s *DefaultUdpSession) Encrypt(buf []byte) ([]byte, error) {
	// buf: [12B nonce space | payload ...]
	if len(buf) < chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("encrypt: buffer too short: %d", len(buf))
	}

	if err := s.SendNonce.incrementNonce(); err != nil {
		return nil, err
	}

	// 1) write nonce into the first 12 bytes
	nonce := buf[:chacha20poly1305.NonceSize]
	_ = s.SendNonce.Encode(nonce)

	// 2) build AAD = sessionId || direction || nonce
	aad := s.CreateAAD(s.isServer, nonce, s.encryptionAadBuf[:])

	// 3) plaintext is everything after the 12B header
	plain := buf[chacha20poly1305.NonceSize:]

	// 4) in-place encrypt: ciphertext overwrites plaintext region
	//    requires caller to allocate +Overhead capacity
	ct := s.sendCipher.Seal(plain[:0], nonce, plain, aad)

	// 5) return header + ciphertext view
	return buf[:chacha20poly1305.NonceSize+len(ct)], nil
}

func (s *DefaultUdpSession) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("invalid ciphertext: too short (%d bytes long)", len(ciphertext))
	}

	nonceBytes := ciphertext[:chacha20poly1305.NonceSize]
	payloadBytes := ciphertext[chacha20poly1305.NonceSize:]

	//converts nonceBytes to [12]byte with no allocations
	nBErr := s.nonceValidator.Validate(*(*[chacha20poly1305.NonceSize]byte)(unsafe.Pointer(&nonceBytes[0])))
	if nBErr != nil {
		return nil, nBErr
	}

	aad := s.CreateAAD(!s.isServer, nonceBytes[:], s.decryptionAadBuf[:])
	plaintext, err := s.recvCipher.Open(payloadBytes[:0], nonceBytes, payloadBytes, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

func (s *DefaultUdpSession) CreateAAD(isServerToClient bool, nonce, aad []byte) []byte {
	direction := []byte("client-to-server")
	if isServerToClient {
		direction = []byte("server-to-client")
	}

	copy(aad[:32], s.SessionId[:])
	copy(aad[len(s.SessionId[:]):], direction)
	copy(aad[len(s.SessionId[:])+len(direction):], nonce)
	return aad[:len(s.SessionId[:])+len(direction)+len(nonce)]
}
