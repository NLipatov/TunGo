package handshake

import (
	"fmt"
	"tungo/application"
)

type ClientIO interface {
	WriteClientHello(hello ClientHello) error
	ReadServerHello() (ServerHello, error)
	WriteClientSignature(signature []byte) error
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
	marshalledHello, marshalErr := hello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	_, clientHelloWriteErr := c.connection.Write(marshalledHello)
	if clientHelloWriteErr != nil {
		return fmt.Errorf("failed to write client hello: %s", clientHelloWriteErr)
	}

	return nil
}

func (c *DefaultClientIO) ReadServerHello() (ServerHello, error) {
	serverHelloBuffer := make([]byte, 128)
	_, shmErr := c.connection.Read(serverHelloBuffer)
	if shmErr != nil {
		return ServerHello{}, fmt.Errorf("failed to read server hello message")
	}

	var serverHello ServerHello
	unmarshalErr := serverHello.UnmarshalBinary(serverHelloBuffer)
	if unmarshalErr != nil {
		return ServerHello{}, unmarshalErr
	}

	return serverHello, nil
}

func (c *DefaultClientIO) WriteClientSignature(signature []byte) error {
	cS, generateKeyErr := (&ClientSignature{}).Write(&signature)
	if generateKeyErr != nil {
		return fmt.Errorf("failed to create client signature message: %s", generateKeyErr)
	}

	_, csErr := c.connection.Write(*cS)
	if csErr != nil {
		return fmt.Errorf("failed to send client signature message: %s", csErr)
	}

	return nil
}
