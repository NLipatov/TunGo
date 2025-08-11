package handshake

import (
	"fmt"
	"net"
	"tungo/application"
	"tungo/infrastructure/network"
	"tungo/infrastructure/settings"
)

const (
	lengthHeaderLength      = 2
	signatureLength         = 64
	nonceLength             = 32
	curvePublicKeyLength    = 32
	minIpLength             = 4
	maxIpLength             = 39
	MaxClientHelloSizeBytes = maxIpLength + lengthHeaderLength + curvePublicKeyLength + curvePublicKeyLength + nonceLength
	minClientHelloSizeBytes = minIpLength + lengthHeaderLength + curvePublicKeyLength + curvePublicKeyLength + nonceLength
)

type DefaultHandshake struct {
	id                                  [32]byte
	clientKey                           []byte
	serverKey                           []byte
	Ed25519PublicKey, Ed25519PrivateKey []byte // client will only have public key
}

func NewHandshake(
	Ed25519PublicKey, Ed25519PrivateKey []byte,
) *DefaultHandshake {
	return &DefaultHandshake{
		Ed25519PublicKey:  Ed25519PublicKey,
		Ed25519PrivateKey: Ed25519PrivateKey,
	}
}

func (h *DefaultHandshake) Id() [32]byte {
	return h.id
}

func (h *DefaultHandshake) ClientKey() []byte {
	return h.clientKey
}

func (h *DefaultHandshake) ServerKey() []byte {
	return h.serverKey
}

func (h *DefaultHandshake) ServerSideHandshake(conn application.ConnectionAdapter) (net.IP, error) {
	c := newDefaultCrypto()

	// Generate server hello response
	curvePublic, curvePrivate, curveErr := c.GenerateX25519KeyPair()
	if curveErr != nil {
		return nil, curveErr
	}
	serverNonce := c.GenerateRandomBytesArray(32)

	//handshake process starts here
	handshake := NewServerHandshake(
		network.NewTcpFullWriteAdapter(conn),
	)
	clientHello, clientHelloErr := handshake.ReceiveClientHello()
	if clientHelloErr != nil {
		return nil, clientHelloErr
	}

	serverHelloErr := handshake.SendServerHello(c, h.Ed25519PrivateKey, serverNonce, curvePublic, clientHello.nonce)
	if serverHelloErr != nil {
		return nil, serverHelloErr
	}

	signatureErr := handshake.VerifyClientSignature(c, clientHello, serverNonce)
	if signatureErr != nil {
		return nil, signatureErr
	}

	sessionId, clientToServerKey, serverToClientKey, sessionKeysErr := c.GenerateChaCha20KeysServerside(curvePrivate[:], serverNonce, &clientHello)
	if sessionKeysErr != nil {
		return nil, sessionKeysErr
	}

	h.id = sessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return clientHello.ipAddress, nil
}

func (h *DefaultHandshake) ClientSideHandshake(conn application.ConnectionAdapter, settings settings.Settings) error {
	c := newDefaultCrypto()

	edPublicKey, edPrivateKey, generateKeyErr := c.GenerateEd25519KeyPair()
	if generateKeyErr != nil {
		return fmt.Errorf("failed to generate ed25519 key pair: %s", generateKeyErr)
	}

	// create session key pair
	sessionPublicKey, sessionPrivateKey, sessionKeyPairErr := c.GenerateX25519KeyPair()
	if sessionKeyPairErr != nil {
		return sessionKeyPairErr
	}

	clientNonce := c.GenerateRandomBytesArray(32)

	clientIO := NewDefaultClientIO(
		network.NewTcpFullWriteAdapter(conn),
	)
	handshake := NewClientHandshake(conn, clientIO, c)
	helloErr := handshake.SendClientHello(settings, edPublicKey, sessionPublicKey, clientNonce)
	if helloErr != nil {
		return helloErr
	}

	serverHello, serverHelloErr := handshake.ReceiveServerHello()
	if serverHelloErr != nil {
		return serverHelloErr
	}

	sendSignatureErr := handshake.SendSignature(h.Ed25519PublicKey, edPrivateKey, sessionPublicKey, serverHello, clientNonce)
	if sendSignatureErr != nil {
		return sendSignatureErr
	}

	serverToClientKey, clientToServerKey, derivedSessionId, calculateKeysErr := c.GenerateChaCha20KeysClientside(sessionPrivateKey[:], clientNonce, &serverHello)
	if calculateKeysErr != nil {
		return calculateKeysErr
	}

	h.id = derivedSessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return nil
}
