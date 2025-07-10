package handshake

import (
	"crypto/ed25519"
	"fmt"
)

type ServerHandshake struct {
	io ServerIO
}

func NewServerHandshake(io ServerIO) ServerHandshake {
	return ServerHandshake{
		io: io,
	}
}

func (h *ServerHandshake) ReceiveClientHello() (ClientHello, error) {
	return h.io.ReceiveClientHello()
}

func (h *ServerHandshake) SendServerHello(
	c Crypto,
	serverPrivateKey ed25519.PrivateKey,
	serverNonce []byte,
	curvePublic,
	clientNonce []byte) error {
	serverDataToSign := append(append(curvePublic, serverNonce...), clientNonce...)
	serverSignature := c.Sign(serverPrivateKey, serverDataToSign)
	serverHello := NewServerHello(serverSignature, serverNonce, curvePublic)

	return h.io.SendServerHello(serverHello)
}

func (h *ServerHandshake) VerifyClientSignature(c Crypto, hello ClientHello, serverNonce []byte) error {
	signature, err := h.io.ReadClientSignature()
	if err != nil {
		return err
	}

	// Verify client signature
	if !c.Verify(hello.edPublicKey, append(append(hello.curvePublicKey, hello.nonce...), serverNonce...), signature.data) {
		return fmt.Errorf("client failed signature verification")
	}

	return nil
}
