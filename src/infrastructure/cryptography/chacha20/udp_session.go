package chacha20

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"unsafe"
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
		nonceBuf         *Sliding64
		encryptionAadBuf [60]byte //32 bytes for sessionId, 16 bytes for direction, 12 bytes for nonce. 60 bytes total.
		decryptionAadBuf [60]byte //32 bytes for sessionId, 16 bytes for direction, 12 bytes for nonce. 60 bytes total.
	}
)

func NewUdpSession(id [32]byte, sendKey, recvKey []byte, isServer bool, nonceBufferSize int) (*DefaultUdpSession, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}

	return &DefaultUdpSession{
		SessionId:  id,
		sendCipher: sendCipher,
		recvCipher: recvCipher,
		RecvNonce:  NewNonce(),
		SendNonce:  NewNonce(),
		isServer:   isServer,
		nonceBuf:   NewSliding64(),
		encoder:    DefaultUDPEncoder{},
	}, nil
}

func (s *DefaultUdpSession) Encrypt(data []byte) ([]byte, error) {
	packageLen := binary.BigEndian.Uint32(data[:12])
	plaintext := data[12 : 12+packageLen]
	err := s.SendNonce.incrementNonce()
	if err != nil {
		return nil, err
	}

	_ = s.SendNonce.Encode(data[:12])

	aad := s.CreateAAD(s.isServer, data[:12], s.encryptionAadBuf[:])
	ciphertext := s.sendCipher.Seal(plaintext[:0], data[:12], plaintext, aad)

	return data[:len(ciphertext)+12], nil
}

func (s *DefaultUdpSession) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 12 {
		return nil, fmt.Errorf("invalid ciphertext: too short (%d bytes long)", len(ciphertext))
	}

	nonceBytes := ciphertext[:12]
	payloadBytes := ciphertext[12:]

	//converts nonceBytes to [12]byte with no allocations
	nBErr := s.nonceBuf.Validate(*(*[12]byte)(unsafe.Pointer(&nonceBytes[0])))
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
