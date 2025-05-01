package handshake

import (
	"tungo/application"
)

type ServerHandshake struct {
	conn application.ConnectionAdapter
}

func NewServerHandshake(conn application.ConnectionAdapter) ServerHandshake {
	return ServerHandshake{
		conn: conn,
	}
}

func (h *ServerHandshake) ReceiveClientHello() (ClientHello, error) {
	buf := make([]byte, MaxClientHelloSizeBytes)
	_, readErr := h.conn.Read(buf)
	if readErr != nil {
		return ClientHello{}, readErr
	}

	//Read client hello
	var clientHello ClientHello
	unmarshalErr := clientHello.UnmarshalBinary(buf)
	if unmarshalErr != nil {
		return ClientHello{}, unmarshalErr
	}

	return clientHello, nil
}

func (h *ServerHandshake) SendServerHello() {}
