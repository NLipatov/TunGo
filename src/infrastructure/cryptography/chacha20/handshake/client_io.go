package handshake

import (
	"fmt"
	"net"
	"tungo/settings"

	"golang.org/x/crypto/ed25519"
)

type ClientIO interface {
	WriteClientHello() error
	ReadServerHello() (ServerHello, error)
	WriteClientSignature(signature []byte) error
}

type DefaultClientIO struct {
	connection       net.Conn
	settings         settings.ConnectionSettings
	ed25519PublicKey []byte
	sessionPublicKey []byte
	randomSalt       []byte
}

func NewDefaultClientIO(connection net.Conn, settings settings.ConnectionSettings, ed25519PublicKey ed25519.PublicKey, sessionPublicKey []byte, randomSalt []byte) ClientIO {
	return &DefaultClientIO{
		connection:       connection,
		settings:         settings,
		ed25519PublicKey: ed25519PublicKey,
		sessionPublicKey: sessionPublicKey,
		randomSalt:       randomSalt,
	}
}

func (c *DefaultClientIO) WriteClientHello() error {
	hello := NewClientHello(4, c.settings.InterfaceAddress, c.ed25519PublicKey, c.sessionPublicKey, c.randomSalt)
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
