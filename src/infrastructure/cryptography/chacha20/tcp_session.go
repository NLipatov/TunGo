package chacha20

import (
	"crypto/cipher"
	"tungo/application"
	"unsafe"

	"golang.org/x/crypto/chacha20poly1305"
)

type DefaultTcpSession struct {
	sendCipher         cipher.AEAD
	recvCipher         cipher.AEAD
	SendNonce          *Nonce
	RecvNonce          *Nonce
	isServer           bool
	SessionId          [32]byte
	nonceValidator     *StrictCounter
	encryptionAadBuf   []byte
	decryptionAadBuf   []byte
	encryptionNonceBuf [12]byte
	decryptionNonceBuf [12]byte
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
		RecvNonce:          NewNonce(),
		SendNonce:          NewNonce(),
		isServer:           isServer,
		nonceValidator:     NewStrictCounter(),
		encryptionNonceBuf: [12]byte{},
		decryptionNonceBuf: [12]byte{},
		encryptionAadBuf:   make([]byte, 80),
		decryptionAadBuf:   make([]byte, 80),
	}, nil
}

func (s *DefaultTcpSession) FromHandshake(handshake application.Handshake, isServer bool) (application.CryptographyService, error) {
	sendCipher, err := chacha20poly1305.New(handshake.ClientKey())
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(handshake.ServerKey())
	if err != nil {
		return nil, err
	}

	return &DefaultTcpSession{
		SessionId:          handshake.Id(),
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          NewNonce(),
		SendNonce:          NewNonce(),
		isServer:           isServer,
		nonceValidator:     NewStrictCounter(),
		encryptionNonceBuf: [12]byte{},
		decryptionNonceBuf: [12]byte{},
		encryptionAadBuf:   make([]byte, 80),
		decryptionAadBuf:   make([]byte, 80),
	}, nil
}

func (s *DefaultTcpSession) Encrypt(plaintext []byte) ([]byte, error) {
	err := s.SendNonce.incrementNonce()
	if err != nil {
		return nil, err
	}

	nonceBytes := s.SendNonce.Encode(s.encryptionNonceBuf[:])

	aad := s.CreateAAD(s.isServer, nonceBytes, s.encryptionAadBuf)
	ciphertext := s.sendCipher.Seal(plaintext[:0], nonceBytes, plaintext, aad)

	return ciphertext, nil
}

func (s *DefaultTcpSession) Decrypt(ciphertext []byte) ([]byte, error) {
	err := s.RecvNonce.incrementNonce()
	if err != nil {
		return nil, err
	}

	nonceBytes := s.RecvNonce.Encode(s.decryptionNonceBuf[:])

	//converts nonceBytes to [12]byte with no allocations
	nBErr := s.nonceValidator.Validate(*(*[12]byte)(unsafe.Pointer(&nonceBytes[0])))
	if nBErr != nil {
		return nil, nBErr
	}

	aad := s.CreateAAD(!s.isServer, nonceBytes, s.decryptionAadBuf)
	plaintext, err := s.recvCipher.Open(ciphertext[:0], nonceBytes, ciphertext, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, err
	}

	return plaintext, nil
}

func (s *DefaultTcpSession) CreateAAD(isServerToClient bool, nonce, aad []byte) []byte {
	direction := []byte("client-to-server")
	if isServerToClient {
		direction = []byte("server-to-client")
	}

	copy(aad, s.SessionId[:])
	copy(aad[len(s.SessionId):], direction)
	copy(aad[len(s.SessionId)+len(direction):], nonce)
	return aad[:len(s.SessionId)+len(direction)+len(nonce)]
}
