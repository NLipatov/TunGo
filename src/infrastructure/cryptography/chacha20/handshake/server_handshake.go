package handshake

import (
	"crypto/ed25519"
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

func (h *ServerHandshake) SendServerHello(
	serverPrivateKey ed25519.PrivateKey,
	serverNonce []byte,
	curvePublic,
	clientNonce []byte) error {
	c := newDefaultCrypto()

	serverDataToSign := append(append(curvePublic, serverNonce...), clientNonce...)
	serverSignature := c.Sign(serverPrivateKey, serverDataToSign)
	serverHello := NewServerHello(serverSignature, serverNonce, curvePublic)
	marshalledServerHello, marshalErr := serverHello.MarshalBinary()
	if marshalErr != nil {
		return marshalErr
	}

	_, writeErr := h.conn.Write(marshalledServerHello)
	return writeErr
}
