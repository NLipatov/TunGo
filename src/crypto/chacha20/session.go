package chacha20

import (
	"crypto/cipher"
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"io"
)

type Session struct {
	sendCipher cipher.AEAD
	recvCipher cipher.AEAD
	SendNonce  *Nonce
	RecvNonce  *Nonce
	isServer   bool
	SessionId  [32]byte
	nonceBuf   *NonceBuf
}

func DeriveSessionId(sharedSecret []byte, salt []byte) ([32]byte, error) {
	var sessionID [32]byte

	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt, []byte("session-id-derivation"))
	if _, err := io.ReadFull(hkdfReader, sessionID[:]); err != nil {
		return [32]byte{}, fmt.Errorf("failed to derive session ID: %w", err)
	}

	return sessionID, nil
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
		RecvNonce:  NewNonce(),
		SendNonce:  NewNonce(),
		isServer:   isServer,
		nonceBuf:   nil,
	}, nil
}

func (s *Session) UseNonceRingBuffer(size int) *Session {
	if size < 1024 {
		size = 1024
	}

	s.nonceBuf = NewNonceBuf(size)
	return s
}

func (s *Session) Encrypt(plaintext []byte) ([]byte, *Nonce, error) {
	err := s.SendNonce.incrementNonce()
	if err != nil {
		return nil, nil, err
	}

	nonceBytes := s.SendNonce.Encode()

	aad := s.CreateAAD(s.isServer, nonceBytes)
	ciphertext := s.sendCipher.Seal(plaintext[:0], nonceBytes, plaintext, aad)

	return ciphertext, s.SendNonce, nil
}

func (s *Session) Decrypt(ciphertext []byte) ([]byte, *Nonce, error) {
	err := s.RecvNonce.incrementNonce()
	if err != nil {
		return nil, nil, err
	}

	nonceBytes := s.RecvNonce.Encode()

	aad := s.CreateAAD(!s.isServer, nonceBytes)
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonceBytes, ciphertext, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, nil, err
	}

	return plaintext, s.RecvNonce, nil
}

func (s *Session) DecryptViaNonceBuf(ciphertext []byte, nonce *Nonce) ([]byte, uint32, uint64, error) {
	nBErr := s.nonceBuf.Insert(nonce)
	if nBErr != nil {
		return nil, 0, 0, nBErr
	}

	nonceBytes := nonce.Encode()
	aad := s.CreateAAD(!s.isServer, nonceBytes[:])
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonceBytes, ciphertext, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, nonce.high, nonce.low, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nonce.high, nonce.low, nil
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
