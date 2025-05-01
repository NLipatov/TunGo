package handshake

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
	"io"
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
	c crypto,
	serverPrivateKey ed25519.PrivateKey,
	serverNonce []byte,
	curvePublic,
	clientNonce []byte) error {
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

func (h *ServerHandshake) VerifyClientSignature(c crypto, hello ClientHello, serverNonce []byte) error {
	clientSignatureBuf := make([]byte, 64)

	// Read client signature
	_, readErr := h.conn.Read(clientSignatureBuf)
	if readErr != nil {
		return readErr
	}
	var clientSignature Signature
	unmarshalErr := clientSignature.UnmarshalBinary(clientSignatureBuf)
	if unmarshalErr != nil {
		return unmarshalErr
	}

	// Verify client signature
	if !c.Verify(hello.edPublicKey, append(append(hello.curvePublicKey, hello.clientNonce...), serverNonce...), clientSignature.Signature) {
		return fmt.Errorf("client failed signature verification")
	}

	return nil
}

func (h *ServerHandshake) CalculateKeys(
	curvePrivate,
	serverNonce []byte,
	hello ClientHello) (sessionId, clientToServerKey, serverToClientKey []byte, err error) {
	// Generate shared secret and salt
	sharedSecret, _ := curve25519.X25519(curvePrivate[:], hello.curvePublicKey)
	salt := sha256.Sum256(append(serverNonce, hello.clientNonce...))

	infoSC := []byte("server-to-client") // server-key info
	infoCS := []byte("client-to-server") // client-key info

	// Generate HKDF for both encryption directions
	serverToClientHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoSC)
	clientToServerHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoCS)
	keySize := chacha20poly1305.KeySize

	// Generate keys for both encryption directions
	serverToClientKey = make([]byte, keySize)
	_, _ = io.ReadFull(serverToClientHKDF, serverToClientKey)
	clientToServerKey = make([]byte, keySize)
	_, _ = io.ReadFull(clientToServerHKDF, clientToServerKey)

	derivedSessionId, deriveSessionIdErr := deriveSessionId(sharedSecret, salt[:])
	if deriveSessionIdErr != nil {
		return nil,
			nil,
			nil,
			fmt.Errorf("failed to derive session id: %s", derivedSessionId)
	}

	return sessionId, clientToServerKey, serverToClientKey, nil
}
