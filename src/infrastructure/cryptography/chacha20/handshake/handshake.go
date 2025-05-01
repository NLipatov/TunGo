package handshake

import (
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/curve25519"
	"io"
	"net"
	"tungo/application"
	"tungo/settings"
	"tungo/settings/client_configuration"
	"tungo/settings/server_configuration"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
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
	c := newDefaultCrypto()
	serverConfigurationManager := server_configuration.NewManager(server_configuration.NewServerResolver())
	conf, err := serverConfigurationManager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %s", err)
	}

	handshake := NewServerHandshake(conn)
	clientHello, clientHelloErr := handshake.ReceiveClientHello()
	if clientHelloErr != nil {
		return nil, clientHelloErr
	}

	// Generate server hello response
	curvePublic, curvePrivate, curveErr := c.GenerateX25519KeyPair()
	if curveErr != nil {
		return nil, curveErr
	}
	serverNonce := c.GenerateRandomBytesArray(32)

	serverHelloErr := handshake.SendServerHello(c, conf.Ed25519PrivateKey, serverNonce, curvePublic, clientHello.clientNonce)
	if serverHelloErr != nil {
		return nil, serverHelloErr
	}

	clientSignatureBuf := make([]byte, 64)

	// Read client signature
	_, err = conn.Read(clientSignatureBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read client signature: %s\n", err)
	}
	var clientSignature Signature
	signatureErr := clientSignature.UnmarshalBinary(clientSignatureBuf)
	if signatureErr != nil {
		return nil, signatureErr
	}

	// Verify client signature
	if !c.Verify(clientHello.edPublicKey, append(append(clientHello.curvePublicKey, clientHello.clientNonce...), serverNonce...), clientSignature.Signature) {
		return nil, fmt.Errorf("client signature verification failed")
	}

	// Generate shared secret and salt
	sharedSecret, _ := curve25519.X25519(curvePrivate[:], clientHello.curvePublicKey)
	salt := sha256.Sum256(append(serverNonce, clientHello.clientNonce...))

	infoSC := []byte("server-to-client") // server-key info
	infoCS := []byte("client-to-server") // client-key info

	// Generate HKDF for both encryption directions
	serverToClientHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoSC)
	clientToServerHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoCS)
	keySize := chacha20poly1305.KeySize

	// Generate keys for both encryption directions
	serverToClientKey := make([]byte, keySize)
	_, _ = io.ReadFull(serverToClientHKDF, serverToClientKey)
	clientToServerKey := make([]byte, keySize)
	_, _ = io.ReadFull(clientToServerHKDF, clientToServerKey)

	derivedSessionId, deriveSessionIdErr := deriveSessionId(sharedSecret, salt[:])
	if deriveSessionIdErr != nil {
		return nil, fmt.Errorf("failed to derive session id: %s", derivedSessionId)
	}

	h.id = derivedSessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return &clientHello.ipAddress, nil
}

func (h *HandshakeImpl) ClientSideHandshake(conn net.Conn, settings settings.ConnectionSettings) error {
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
	clientHello := NewClientHello(4, settings.InterfaceAddress, edPublicKey, sessionPublicKey, sessionSalt)
	clientHelloWriteErr := clientIO.WriteClientHello(clientHello)
	if clientHelloWriteErr != nil {
		return clientHelloWriteErr
	}

	serverHello, readServerHelloErr := clientIO.ReadServerHello()
	if readServerHelloErr != nil {
		return readServerHelloErr
	}

	if !c.Verify(clientConf.Ed25519PublicKey, append(append(serverHello.CurvePublicKey, serverHello.Nonce...), sessionSalt...), serverHello.Signature) {
		return fmt.Errorf("server failed signature check")
	}

	dataToSign := append(append(sessionPublicKey, sessionSalt...), serverHello.Nonce...)
	signature := NewSignature(c.Sign(edPrivateKey, dataToSign))
	writeSignatureErr := clientIO.WriteClientSignature(signature)
	if writeSignatureErr != nil {
		return writeSignatureErr
	}

	serverToClientKey, clientToServerKey, derivedSessionId, calculateKeysErr := clientCrypto.CalculateKeys(sessionPrivateKey[:], sessionSalt, serverHello.Nonce, serverHello.CurvePublicKey)
	if calculateKeysErr != nil {
		return calculateKeysErr
	}

	h.id = derivedSessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return nil
}
