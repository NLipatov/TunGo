package ChaCha20

import (
	"crypto/cipher"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
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
		nonceBuf:   NewNonceBuf(1024),
	}, nil
}

func (s *Session) Encrypt(plaintext []byte) ([]byte, uint32, uint64, error) {
	nonce, high, low, err := s.SendNonce.incrementNonce()
	if err != nil {
		return nil, high, low, err
	}
	aad := s.CreateAAD(s.isServer, nonce)
	ciphertext := s.sendCipher.Seal(plaintext[:0], nonce, plaintext, aad)

	return ciphertext, high, low, nil
}

func (s *Session) Decrypt(ciphertext []byte) ([]byte, uint32, uint64, error) {
	nonce, high, low, err := s.RecvNonce.incrementNonce()
	if err != nil {
		return nil, high, low, err
	}
	aad := s.CreateAAD(!s.isServer, nonce)
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonce, ciphertext, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, high, low, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, high, low, nil
}

func (s *Session) DecryptViaNonceBuf(ciphertext []byte, nonce Nonce) ([]byte, uint32, uint64, error) {
	nBErr := s.nonceBuf.Insert(nonce)
	if nBErr != nil {
		return nil, 0, 0, nBErr
	}

	nonceBytes := Encode(nonce.High, nonce.Low)
	aad := s.CreateAAD(!s.isServer, nonceBytes[:])
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonceBytes[:], ciphertext, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, nonce.High, nonce.Low, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nonce.High, nonce.Low, nil
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
