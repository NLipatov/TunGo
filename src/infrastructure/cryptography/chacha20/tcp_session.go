package chacha20

import (
	"crypto/cipher"
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"io"
)

type TcpCryptographyService struct {
	sendCipher         cipher.AEAD
	recvCipher         cipher.AEAD
	SendNonce          *Nonce
	RecvNonce          *Nonce
	isServer           bool
	SessionId          [32]byte
	nonceBuf           *NonceCounter
	encryptionAadBuf   []byte
	decryptionAadBuf   []byte
	encryptionNonceBuf [12]byte
	decryptionNonceBuf [12]byte
}

func DeriveSessionId(sharedSecret []byte, salt []byte) ([32]byte, error) {
	var sessionID [32]byte

	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt, []byte("session-id-derivation"))
	if _, err := io.ReadFull(hkdfReader, sessionID[:]); err != nil {
		return [32]byte{}, fmt.Errorf("failed to derive session ID: %w", err)
	}

	return sessionID, nil
}

func NewTcpCryptographyService(id [32]byte, sendKey, recvKey []byte, isServer bool) (*TcpCryptographyService, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}

	return &TcpCryptographyService{
		SessionId:          id,
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          NewNonce(),
		SendNonce:          NewNonce(),
		isServer:           isServer,
		nonceBuf:           nil,
		encryptionNonceBuf: [12]byte{},
		decryptionNonceBuf: [12]byte{},
		encryptionAadBuf:   make([]byte, 80),
		decryptionAadBuf:   make([]byte, 80),
	}, nil
}

func (s *TcpCryptographyService) UseNonceRingBuffer() *TcpCryptographyService {
	return s
}

func (s *TcpCryptographyService) Encrypt(plaintext []byte) ([]byte, error) {
	err := s.SendNonce.incrementNonce()
	if err != nil {
		return nil, err
	}

	nonceBytes := s.SendNonce.Encode(s.encryptionNonceBuf[:])

	aad := s.CreateAAD(s.isServer, nonceBytes, s.encryptionAadBuf)
	ciphertext := s.sendCipher.Seal(plaintext[:0], nonceBytes, plaintext, aad)

	return ciphertext, nil
}

func (s *TcpCryptographyService) Decrypt(ciphertext []byte) ([]byte, error) {
	err := s.RecvNonce.incrementNonce()
	if err != nil {
		return nil, err
	}

	nonceBytes := s.RecvNonce.Encode(s.decryptionNonceBuf[:])

	aad := s.CreateAAD(!s.isServer, nonceBytes, s.decryptionAadBuf)
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonceBytes, ciphertext, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, err
	}

	return plaintext, nil
}

func (s *TcpCryptographyService) CreateAAD(isServerToClient bool, nonce, aad []byte) []byte {
	direction := []byte("client-to-server")
	if isServerToClient {
		direction = []byte("server-to-client")
	}

	copy(aad, s.SessionId[:])
	copy(aad[len(s.SessionId):], direction)
	copy(aad[len(s.SessionId)+len(direction):], nonce)
	return aad[:len(s.SessionId)+len(direction)+len(nonce)]
}
