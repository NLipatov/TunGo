package handshake

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"tungo/application"
)

type ServerHandshake struct {
	adapter application.ConnectionAdapter
}

func NewServerHandshake(adapter application.ConnectionAdapter) ServerHandshake {
	return ServerHandshake{
		adapter: adapter,
	}
}

func (h *ServerHandshake) ReceiveClientHello() (ClientHello, error) {
	// read client hello to buf
	buf := make([]byte, MaxClientHelloSizeBytes)
	n, rErr := h.adapter.Read(buf)
	if rErr != nil {
		return ClientHello{}, rErr
	}

	// deserialize client hello from buf
	hello := NewEmptyClientHelloWithDefaultIPValidator()
	uErr := hello.UnmarshalBinary(buf[:n])
	if uErr != nil {
		return ClientHello{}, uErr
	}

	return hello, nil
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
	marshalledServerHello, marshalErr := serverHello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	_, writeErr := h.adapter.Write(marshalledServerHello)
	return writeErr
}

func (h *ServerHandshake) VerifyClientSignature(c Crypto, hello ClientHello, serverNonce []byte) error {
	clientSignatureBuf := make([]byte, 64)
	if _, err := io.ReadFull(h.adapter, clientSignatureBuf); err != nil {
		return err
	}

	var clientSignature Signature
	unmarshalErr := clientSignature.UnmarshalBinary(clientSignatureBuf)
	if unmarshalErr != nil {
		return unmarshalErr
	}

	// Verify client signature
	if !c.Verify(hello.edPublicKey, append(append(hello.curvePublicKey, hello.nonce...), serverNonce...), clientSignature.data) {
		return fmt.Errorf("client failed signature verification")
	}

	return nil
}
