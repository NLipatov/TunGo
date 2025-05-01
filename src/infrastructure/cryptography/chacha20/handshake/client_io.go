package handshake

import (
	"tungo/application"
)

type ClientIO interface {
	WriteClientHello(hello ClientHello) error
	ReadServerHello() (ServerHello, error)
	WriteClientSignature(signature Signature) error
}

type DefaultClientIO struct {
	connection application.ConnectionAdapter
}

func NewDefaultClientIO(connection application.ConnectionAdapter) ClientIO {
	return &DefaultClientIO{
		connection: connection,
	}
}

func (c *DefaultClientIO) WriteClientHello(hello ClientHello) error {
	data, marshalErr := hello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	_, writeErr := c.connection.Write(data)
	if writeErr != nil {
		return writeErr
	}

	return nil
}

func (c *DefaultClientIO) ReadServerHello() (ServerHello, error) {
	buffer := make([]byte, 128)
	_, readErr := c.connection.Read(buffer)
	if readErr != nil {
		return ServerHello{}, readErr
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

	_, writeErr := c.connection.Write(data)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
