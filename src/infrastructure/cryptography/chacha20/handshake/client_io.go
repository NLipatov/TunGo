package handshake

import (
	"fmt"
	"net"
	"tungo/settings"

	"golang.org/x/crypto/ed25519"
)

type ClientIO interface {
	SendClientHello() error
	ReceiveServerHello() (ServerHello, error)
	SendClientSignature(signature []byte) error
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

func (c *DefaultClientIO) SendClientHello() error {
	clientHello, clientHelloErr := NewClientHello(4, c.settings.InterfaceAddress, c.ed25519PublicKey, c.sessionPublicKey, c.randomSalt)
	if clientHelloErr != nil {
		return clientHelloErr
	}

	clientHelloBytes, marshalErr := clientHello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	_, clientHelloWriteErr := c.connection.Write(clientHelloBytes)
	if clientHelloWriteErr != nil {
		return fmt.Errorf("failed to write client hello: %s", clientHelloWriteErr)
	}

	return nil
}

func (c *DefaultClientIO) ReceiveServerHello() (ServerHello, error) {
	serverHelloBuffer := make([]byte, 128)
	_, shmErr := c.connection.Read(serverHelloBuffer)
	if shmErr != nil {
		return ServerHello{}, fmt.Errorf("failed to read server hello message")
	}

	var serverHello ServerHello
	if err := serverHello.UnmarshalBinary(serverHelloBuffer); err != nil {
		return ServerHello{}, fmt.Errorf("cannot parse server hello: %s", err)
	}

	return serverHello, nil
}

func (c *DefaultClientIO) SendClientSignature(signature []byte) error {
	clientSignature, clientSignatureErr := NewClientSignature(signature)
	if clientSignatureErr != nil {
		return clientSignatureErr
	}

	clientSignatureBytes, clientSignatureBytesErr := clientSignature.MarshalBinary()
	if clientSignatureBytesErr != nil {
		return clientSignatureBytesErr
	}

	_, csErr := c.connection.Write(clientSignatureBytes)
	if csErr != nil {
		return fmt.Errorf("failed to send client signature message: %s", csErr)
	}

	return nil
}
