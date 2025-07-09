package handshake

import (
	"fmt"
	"io"
	"tungo/application"
)

type ClientIO interface {
	WriteClientHello(hello *ClientHello) error
	ReadServerHello() (ServerHello, error)
	WriteClientSignature(signature Signature) error
}

type DefaultClientIO struct {
	connection application.ConnectionAdapter
	obfs       Obfuscator
	enc        Encrypter
}

func NewDefaultClientIO(connection application.ConnectionAdapter, enc Encrypter) ClientIO {
	return &DefaultClientIO{
		connection: connection,
		obfs:       Obfuscator{},
		enc:        enc,
	}
}

func (c *DefaultClientIO) WriteClientHello(hello *ClientHello) error {
	data, marshalErr := hello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	data, marshalErr = c.obfs.Obfuscate(data)
	if marshalErr != nil {
		return marshalErr
	}

	data, marshalErr = c.enc.Encrypt(data)
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
	buffer := make([]byte, signatureLength+nonceLength+curvePublicKeyLength+paddingLengthHeaderBytes+maxPaddingLength+hmacLength)
	n, err := c.connection.Read(buffer)
	if err != nil && err != io.EOF {
		return ServerHello{}, fmt.Errorf("failed to read server hello message: %w", err)
	}
	buffer = buffer[:n]

	plainDec, _, err := c.enc.Decrypt(buffer)
	if err != nil {
		return ServerHello{}, err
	}

	plain, _, err := c.obfs.Deobfuscate(plainDec)
	if err != nil {
		return ServerHello{}, err
	}

	var hello ServerHello
	unmarshalErr := hello.UnmarshalBinary(plain)
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
