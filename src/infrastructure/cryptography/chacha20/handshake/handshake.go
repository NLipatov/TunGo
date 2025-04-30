package handshake

import (
	"crypto/sha256"
	"fmt"
	"net"
	"tungo/application"
	"tungo/settings"
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

type HandshakeImpl struct {
	id        [32]byte
	clientKey []byte
	serverKey []byte
}

func NewHandshake() *HandshakeImpl {
	return &HandshakeImpl{}
}

func (h *HandshakeImpl) Id() [32]byte {
	return h.id
}

func (h *HandshakeImpl) ClientKey() []byte {
	return h.clientKey
}

func (h *HandshakeImpl) ServerKey() []byte {
	return h.serverKey
}

func (h *HandshakeImpl) ServerSideHandshake(conn application.ConnectionAdapter) (*string, error) {
	serverConfigurationManager := server_configuration.NewManager(server_configuration.NewServerResolver())
	conf, err := serverConfigurationManager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %s", err)
	}

	serverCrypto := NewDefaultServerCrypto()

	serverIo := NewDefaultServerIO(conn)
	clientHello, clientHelloErr := serverIo.ReceiveClientHello()
	if clientHelloErr != nil {
		return nil, clientHelloErr
	}

	sessionPublicKey, sessionPrivateKey, sessionKeyErr := serverCrypto.NewX25519SessionKeyPair()
	if sessionKeyErr != nil {
		return nil, sessionKeyErr
	}

	serverNonce := serverCrypto.GenerateNonce()
	serverSignature := serverCrypto.Sign(conf.Ed25519PrivateKey, append(append(sessionPublicKey, serverNonce...), clientHello.ClientNonce...))

	serverHello, serverHelloErr := NewServerHello(serverSignature, serverNonce, sessionPublicKey)
	if serverHelloErr != nil {
		return nil, fmt.Errorf("failed to create server hello: %s", serverHelloErr)
	}

	serverIo.SendServerHello(serverHello)
	clientSignature, clientSignatureErr := serverIo.ReceiveClientSignature()
	if clientSignatureErr != nil {
		return nil, clientSignatureErr
	}

	serverCrypto.Verify(clientHello.Ed25519PubKey, append(append(clientHello.CurvePubKey, clientHello.ClientNonce...), serverNonce...), clientSignature.Signature)

	// Generate shared secret and salt
	sharedSecret, _ := serverCrypto.GenerateSharedSecret(sessionPrivateKey[:], clientHello.CurvePubKey)
	salt := sha256.Sum256(append(serverNonce, clientHello.ClientNonce...))
	serverToClientKey, clientToServerKey, calculateKeysErr := serverCrypto.CalculateKeys(sessionPrivateKey[:], salt[:], serverNonce, clientHello.ClientNonce, clientHello.CurvePubKey, sharedSecret)
	if calculateKeysErr != nil {
		return nil, calculateKeysErr
	}

	sessionDeriver := NewDefaultSessionIdDeriver(sharedSecret, salt[:])

	derivedSessionId, deriveSessionIdErr := sessionDeriver.Derive()
	if deriveSessionIdErr != nil {
		return nil, fmt.Errorf("failed to derive session id: %s", derivedSessionId)
	}

	h.id = derivedSessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return &clientHello.IpAddress, nil
}

func (h *HandshakeImpl) ClientSideHandshake(conn net.Conn, cfg settings.ConnectionSettings) error {
	// prepare kets
	edPub, edPriv, sessPub, sessPriv, salt, err := prepareClientCredentials()
	if err != nil {
		return err
	}

	// send ClientHello
	if err := sendClientHello(conn, cfg.InterfaceAddress, edPub, sessPub, salt); err != nil {
		return err
	}

	// receive ServerHello
	sh, err := receiveAndVerifyServerHello(conn, edPub, salt)
	if err != nil {
		return err
	}

	// sign and send signature
	if err := signAndSendClientSignature(conn, edPriv, sessPub, salt, sh.Nonce); err != nil {
		return err
	}

	// calculate shared keys and session id
	return h.finishKeysAndID(sessPriv, salt, sh)
}
