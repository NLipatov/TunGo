package handshake

import (
	"crypto/rand"
	"fmt"
	"tungo/application"
)

type ObfuscatedClientIO struct {
	io         ClientIO
	connection application.ConnectionAdapter
}

func NewObfuscatedClientIO(
	io ClientIO,
	conn application.ConnectionAdapter,
) ClientIO {
	return &ObfuscatedClientIO{
		io:         io,
		connection: conn,
	}
}

func (c *ObfuscatedClientIO) ReadServerHello() (ServerHello, error) {
	return c.io.ReadServerHello()
}

func (c *ObfuscatedClientIO) WriteClientSignature(signature Signature) error {
	return c.io.WriteClientSignature(signature)
}

func (o *ObfuscatedClientIO) WriteClientHello(hello ClientHello) error {
	data, marshalErr := hello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	if len(data) > ObfuscatedHelloPacketSize {
		return fmt.Errorf("hello too large")
	}

	obfuscatedData := make([]byte, ObfuscatedHelloPacketSize)
	// copy data to obfuscatedData
	copy(obfuscatedData, data)
	// append junk bytes to obfuscatedData
	_, readErr := rand.Read(obfuscatedData[len(data):])
	if readErr != nil {
		return readErr
	}

	_, writeErr := o.connection.Write(obfuscatedData)
	return writeErr
}
