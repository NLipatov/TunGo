package handshake

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"tungo/application/network/connection"
)

type ServerHandshake struct {
	transport connection.Transport
}

func NewServerHandshake(transport connection.Transport) ServerHandshake {
	return ServerHandshake{
		transport: transport,
	}
}

func (h *ServerHandshake) ReceiveClientHello() (ClientHello, error) {
	header := make([]byte, lengthHeaderLength)
	if _, err := io.ReadFull(h.transport, header); err != nil {
		return ClientHello{}, err
	}

	ipLength := int(header[1])
	totalLength := lengthHeaderLength + ipLength + curvePublicKeyLength + curvePublicKeyLength + nonceLength
	if totalLength > MaxClientHelloSizeBytes {
		return ClientHello{}, fmt.Errorf("invalid Client Hello size: %d", totalLength)
	}

	buf := make([]byte, totalLength)
	copy(buf, header)
	if _, err := io.ReadFull(h.transport, buf[lengthHeaderLength:]); err != nil {
		return ClientHello{}, err
	}

	hello := NewEmptyClientHelloWithDefaultIPValidator()
	if err := hello.UnmarshalBinary(buf); err != nil {
		return ClientHello{}, err
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

	_, writeErr := h.transport.Write(marshalledServerHello)
	return writeErr
}

func (h *ServerHandshake) VerifyClientSignature(c Crypto, hello ClientHello, serverNonce []byte) error {
	clientSignatureBuf := make([]byte, 64)
	if _, err := io.ReadFull(h.transport, clientSignatureBuf); err != nil {
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
