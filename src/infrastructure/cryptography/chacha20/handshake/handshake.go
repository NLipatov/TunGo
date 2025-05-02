package handshake

import (
	"fmt"
	"net"
	"tungo/application"
	"tungo/settings"
	"tungo/settings/client_configuration"
	"tungo/settings/server_configuration"
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

type Handshake interface {
	Id() [32]byte
	ClientKey() []byte
	ServerKey() []byte
	ServerSideHandshake(conn application.ConnectionAdapter) (*string, error)
	ClientSideHandshake(conn net.Conn, settings settings.ConnectionSettings) error
}

type DefaultHandshake struct {
	id        [32]byte
	clientKey []byte
	serverKey []byte
}

func NewHandshake() *DefaultHandshake {
	return &DefaultHandshake{}
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

func (h *DefaultHandshake) ServerSideHandshake(conn application.ConnectionAdapter) (*string, error) {
	c := newDefaultCrypto()

	// Generate server hello response
	curvePublic, curvePrivate, curveErr := c.GenerateX25519KeyPair()
	if curveErr != nil {
		return nil, curveErr
	}
	serverNonce := c.GenerateRandomBytesArray(32)

	serverConfigurationManager := server_configuration.NewManager(server_configuration.NewServerResolver())
	conf, err := serverConfigurationManager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %s", err)
	}

	//handshake process starts here
	handshake := NewServerHandshake(conn)
	clientHello, clientHelloErr := handshake.ReceiveClientHello()
	if clientHelloErr != nil {
		return nil, clientHelloErr
	}

	serverHelloErr := handshake.SendServerHello(c, conf.Ed25519PrivateKey, serverNonce, curvePublic, clientHello.nonce)
	if serverHelloErr != nil {
		return nil, serverHelloErr
	}

	signatureErr := handshake.VerifyClientSignature(c, clientHello, serverNonce)
	if signatureErr != nil {
		return nil, signatureErr
	}

	sessionId, clientToServerKey, serverToClientKey, sessionKeysErr := c.GenerateChaCha20KeysServerside(curvePrivate[:], serverNonce, clientHello)
	if sessionKeysErr != nil {
		return nil, sessionKeysErr
	}

	h.id = sessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return &clientHello.ipAddress, nil
}

func (h *DefaultHandshake) ClientSideHandshake(conn net.Conn, settings settings.ConnectionSettings) error {
	c := newDefaultCrypto()
	configurationManager := client_configuration.NewManager()
	clientConf, generateKeyErr := configurationManager.Configuration()
	if generateKeyErr != nil {
		return fmt.Errorf("failed to read client configuration: %s", generateKeyErr)
	}

	clientCrypto := NewDefaultClientCrypto()

	edPublicKey, edPrivateKey, generateKeyErr := c.GenerateEd25519KeyPair()
	if generateKeyErr != nil {
		return fmt.Errorf("failed to generate ed25519 key pair: %s", generateKeyErr)
	}

	// create session key pair
	sessionPublicKey, sessionPrivateKey, sessionKeyPairErr := c.GenerateX25519KeyPair()
	if sessionKeyPairErr != nil {
		return sessionKeyPairErr
	}

	sessionSalt := c.GenerateRandomBytesArray(32)

	clientIO := NewDefaultClientIO(conn)
	handshake := NewClientHandshake(conn, clientIO, c)
	helloErr := handshake.SendClientHello(settings, edPublicKey, sessionPublicKey, sessionSalt)
	if helloErr != nil {
		return helloErr
	}

	serverHello, serverHelloErr := handshake.ReceiveServerHello()
	if serverHelloErr != nil {
		return serverHelloErr
	}

	sendSignatureErr := handshake.SendSignature(clientConf.Ed25519PublicKey, edPrivateKey, sessionPublicKey, serverHello, sessionSalt)
	if sendSignatureErr != nil {
		return sendSignatureErr
	}

	serverToClientKey, clientToServerKey, derivedSessionId, calculateKeysErr := clientCrypto.CalculateKeys(sessionPrivateKey[:], sessionSalt, serverHello.nonce, serverHello.curvePublicKey)
	if calculateKeysErr != nil {
		return calculateKeysErr
	}

	h.id = derivedSessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return nil
}
