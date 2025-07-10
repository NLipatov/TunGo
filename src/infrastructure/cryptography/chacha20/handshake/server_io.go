package handshake

import (
	"io"
	"tungo/application"
)

type ServerIO interface {
	ReceiveClientHello() (ClientHello, error)
	SendServerHello(hello ServerHello) error
	ReadClientSignature() (Signature, error)
}

type DefaultServerIO struct {
	conn application.ConnectionAdapter
}

func NewDefaultServerIO(conn application.ConnectionAdapter) ServerIO {
	return &DefaultServerIO{
		conn: conn,
	}
}

func (d *DefaultServerIO) ReceiveClientHello() (ClientHello, error) {
	buf := make([]byte, MaxClientHelloSizeBytes)
	n, readErr := d.conn.Read(buf)
	if readErr != nil {
		return ClientHello{}, readErr
	}

	//Read client hello
	var clientHello ClientHello
	unmarshalErr := clientHello.UnmarshalBinary(buf[:n])
	if unmarshalErr != nil {
		return ClientHello{}, unmarshalErr
	}

	return clientHello, nil
}

func (d *DefaultServerIO) SendServerHello(hello ServerHello) error {
	marshalledServerHello, marshalErr := hello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	_, writeErr := d.conn.Write(marshalledServerHello)
	return writeErr
}

func (d *DefaultServerIO) ReadClientSignature() (Signature, error) {
	clientSignatureBuf := make([]byte, 64)
	if _, err := io.ReadFull(d.conn, clientSignatureBuf); err != nil {
		return Signature{}, err
	}

	var clientSignature Signature
	unmarshalErr := clientSignature.UnmarshalBinary(clientSignatureBuf)
	if unmarshalErr != nil {
		return Signature{}, unmarshalErr
	}

	return clientSignature, nil
}
