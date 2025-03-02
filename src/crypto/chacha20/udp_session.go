package chacha20

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
)

type (
	UdpSession interface {
		Encrypt(plaintext []byte) ([]byte, error)
		Decrypt(ciphertext []byte) ([]byte, error)
	}
	DefaultUdpSession struct {
		SessionId  [32]byte
		encoder    DefaultUDPEncoder
		sendCipher cipher.AEAD
		recvCipher cipher.AEAD
		SendNonce  *Nonce
		RecvNonce  *Nonce
		isServer   bool
		nonceBuf   *NonceBuf
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
		nonceBuf:   NewNonceBuf(nonceBufferSize),
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

	nonceEncodingErr := s.SendNonce.InplaceEncode(data[:12])
	if nonceEncodingErr != nil {
		return nil, nonceEncodingErr
	}

	aad := s.CreateAAD(s.isServer, data[:12])
	ciphertext := s.sendCipher.Seal(plaintext[:0], data[:12], plaintext, aad)

	return data[:len(ciphertext)+12], nil
}

func (s *DefaultUdpSession) Decrypt(ciphertext []byte) ([]byte, error) {
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

func (s *DefaultUdpSession) CreateAAD(isServerToClient bool, nonce []byte) []byte {
	direction := []byte("client-to-server")
	if isServerToClient {
		direction = []byte("server-to-client")
	}

	aad := append(s.SessionId[:], direction...)
	aad = append(aad, nonce...)
	return aad
}
