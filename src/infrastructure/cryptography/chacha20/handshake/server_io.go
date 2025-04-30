package handshake

import (
	"fmt"
	"tungo/application"
)

type ServerIO interface {
	ReceiveClientHello() (ClientHello, error)
	SendServerHello(ServerHello) error
	ReceiveClientSignature() (ClientSignature, error)
}

type DefaultServerIO struct {
	conn application.ConnectionAdapter
}

func NewDefaultServerIO(conn application.ConnectionAdapter) ServerIO {
	return &DefaultServerIO{
		conn: conn,
	}
}

func (s *DefaultServerIO) ReceiveClientHello() (ClientHello, error) {
	buf := make([]byte, MaxClientHelloSizeBytes)
	_, readErr := s.conn.Read(buf)
	if readErr != nil {
		return ClientHello{}, fmt.Errorf("failed to read from client: %v", readErr)
	}

	var clientHello ClientHello
	unmarshalErr := clientHello.UnmarshalBinary(buf)
	if unmarshalErr != nil {
		return ClientHello{}, unmarshalErr
	}

	return clientHello, nil
}

func (s *DefaultServerIO) SendServerHello(serverHello ServerHello) error {
	serverHelloBytes, serverHelloBytesErr := serverHello.MarshalBinary()
	if serverHelloBytesErr != nil {
		return fmt.Errorf("failed to marshal server hello: %s", serverHelloBytesErr)
	}

	_, err := s.conn.Write(serverHelloBytes)
	return err
}

func (s *DefaultServerIO) ReceiveClientSignature() (ClientSignature, error) {
	clientSignatureBuf := make([]byte, 64)

	_, err := s.conn.Read(clientSignatureBuf)
	if err != nil {
		return ClientSignature{}, fmt.Errorf("failed to read client signature: %s", err)
	}

	var clientSignature ClientSignature
	unmarshalErr := clientSignature.UnmarshalBinary(clientSignatureBuf)
	if unmarshalErr != nil {
		return ClientSignature{}, unmarshalErr
	}

	return clientSignature, nil
}
