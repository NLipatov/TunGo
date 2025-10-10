package handshake

import (
	"fmt"
	"io"
	"tungo/application/network/connection"
)

type ClientIO interface {
	WriteClientHello(hello ClientHello) error
	ReadServerHello() (ServerHello, error)
	WriteClientSignature(signature Signature) error
}

type DefaultClientIO struct {
	transport connection.Transport
}

func NewDefaultClientIO(transport connection.Transport) ClientIO {
	return &DefaultClientIO{
		transport: transport,
	}
}

func (c *DefaultClientIO) WriteClientHello(hello ClientHello) error {
	data, marshalErr := hello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	_, writeErr := c.transport.Write(data)
	if writeErr != nil {
		return writeErr
	}

	return nil
}

func (c *DefaultClientIO) ReadServerHello() (ServerHello, error) {
	buffer := make([]byte, signatureLength+nonceLength+curvePublicKeyLength)
	if _, err := io.ReadFull(c.transport, buffer); err != nil {
		return ServerHello{}, fmt.Errorf("failed to read server hello message: %w", err)
	}

	var hello ServerHello
	unmarshalErr := hello.UnmarshalBinary(buffer)
	if unmarshalErr != nil {
		return ServerHello{}, unmarshalErr
	}

	return hello, nil
}

func (c *DefaultClientIO) WriteClientSignature(signature Signature) error {
	data, marshalErr := signature.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	_, writeErr := c.transport.Write(data)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
