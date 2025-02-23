package chacha20

import (
	"crypto/cipher"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
)

type UdpSession struct {
	SessionId  [32]byte
	encoder    UDPEncoder
	sendCipher cipher.AEAD
	recvCipher cipher.AEAD
	SendNonce  *Nonce
	RecvNonce  *Nonce
	isServer   bool
	nonceBuf   *NonceBuf
}

func NewUdpSession(id [32]byte, sendKey, recvKey []byte, isServer bool) (*UdpSession, error) {
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}

	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}

	return &UdpSession{
		SessionId:  id,
		sendCipher: sendCipher,
		recvCipher: recvCipher,
		RecvNonce:  NewNonce(),
		SendNonce:  NewNonce(),
		isServer:   isServer,
		nonceBuf:   nil,
		encoder:    UDPEncoder{},
	}, nil
}

func (s *UdpSession) UseNonceRingBuffer(size int) *UdpSession {
	if size < 1024 {
		size = 1024
	}

	s.nonceBuf = NewNonceBuf(size)
	return s
}

func (s *UdpSession) Encrypt(plaintext []byte) ([]byte, error) {
	err := s.SendNonce.incrementNonce()
	if err != nil {
		return nil, err
	}

	nonceBytes := s.SendNonce.Encode()

	aad := s.CreateAAD(s.isServer, nonceBytes)
	ciphertext := s.sendCipher.Seal(plaintext[:0], nonceBytes, plaintext, aad)

	packet, packetErr := s.encoder.Encode(ciphertext, s.SendNonce)
	if packetErr != nil {
		return nil, packetErr
	}

	return *packet.Payload, nil
}

func (s *UdpSession) Decrypt(ciphertext []byte) ([]byte, error) {
	packet, packetErr := s.encoder.Decode(ciphertext)
	if packetErr != nil {
		return nil, packetErr
	}

	nonceBytes := packet.Nonce.Encode()
	payloadBytes := ciphertext[12:]

	nBErr := s.nonceBuf.Insert(packet.Nonce)
	if nBErr != nil {
		return nil, nBErr
	}
	aad := s.CreateAAD(!s.isServer, nonceBytes[:])
	plaintext, err := s.recvCipher.Open(payloadBytes[:0], nonceBytes, payloadBytes, aad)
	if err != nil {
		// Properly handle failed decryption attempt to avoid reuse of any state
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

func (s *UdpSession) CreateAAD(isServerToClient bool, nonce []byte) []byte {
	direction := []byte("client-to-server")
	if isServerToClient {
		direction = []byte("server-to-client")
	}

	aad := append(s.SessionId[:], direction...)
	aad = append(aad, nonce...)
	return aad
}
