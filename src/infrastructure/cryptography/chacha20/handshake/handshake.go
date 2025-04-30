package handshake

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"tungo/application"
	"tungo/settings"
	"tungo/settings/client_configuration"
	"tungo/settings/server_configuration"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
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
	serverConfigurationManager := server_configuration.NewManager(server_configuration.NewServerResolver())
	conf, err := serverConfigurationManager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to read server configuration: %s", err)
	}

	buf := make([]byte, MaxClientHelloSizeBytes)
	_, err = conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read from client: %v", err)
	}

	//Read client hello
	clientHello, err := (&ClientHello{}).Read(buf)
	if err != nil {
		return nil, fmt.Errorf("invalid client hello: %s", err)
	}

	// Generate server hello response
	var curvePrivate [32]byte
	_, _ = io.ReadFull(rand.Reader, curvePrivate[:])
	curvePublic, _ := curve25519.X25519(curvePrivate[:], curve25519.Basepoint)
	serverNonce := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, serverNonce)
	serverDataToSign := append(append(curvePublic, serverNonce...), clientHello.ClientNonce...)
	privateEd := conf.Ed25519PrivateKey
	serverSignature := ed25519.Sign(privateEd, serverDataToSign)
	serverHello, err := (&ServerHello{}).Write(&serverSignature, &serverNonce, &curvePublic)
	if err != nil {
		return nil, fmt.Errorf("failed to write server hello: %s\n", err)
	}

	// Send server hello
	_, err = conn.Write(*serverHello)
	clientSignatureBuf := make([]byte, 64)

	// Read client signature
	_, err = conn.Read(clientSignatureBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read client signature: %s\n", err)
	}
	clientSignature, err := (&ClientSignature{}).Read(clientSignatureBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read client signature: %s", err)
	}

	// Verify client signature
	if !ed25519.Verify(clientHello.EdPublicKey, append(append(clientHello.CurvePublicKey, clientHello.ClientNonce...), serverNonce...), clientSignature.ClientSignature) {
		return nil, fmt.Errorf("client signature verification failed")
	}

	// Generate shared secret and salt
	sharedSecret, _ := curve25519.X25519(curvePrivate[:], clientHello.CurvePublicKey)
	salt := sha256.Sum256(append(serverNonce, clientHello.ClientNonce...))

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

	return &clientHello.IpAddress, nil
}

func (h *HandshakeImpl) ClientSideHandshake(conn net.Conn, settings settings.ConnectionSettings) error {
	clientCrypto := NewDefaultClientCrypto()

	edPublicKey, edPrivateKey, generateKeyErr := clientCrypto.GenerateEd25519Keys()
	if generateKeyErr != nil {
		return fmt.Errorf("failed to generate ed25519 key pair: %s", generateKeyErr)
	}

	// create session key pair
	sessionPublicKey, sessionPrivateKey, sessionKeyPairErr := clientCrypto.NewX25519SessionKeyPair()
	if sessionKeyPairErr != nil {
		return sessionKeyPairErr
	}

	sessionSalt := clientCrypto.GenerateSalt()

	clientIO := NewDefaultClientIO(conn, settings, edPublicKey, sessionPublicKey, sessionSalt)
	clientHelloWriteErr := clientIO.SendClientHello()
	if clientHelloWriteErr != nil {
		return clientHelloWriteErr
	}

	serverHello, readServerHelloErr := clientIO.ReceiveServerHello()
	if readServerHelloErr != nil {
		return readServerHelloErr
	}

	configurationManager := client_configuration.NewManager()
	clientConf := NewDefaultClientConf(configurationManager)
	serverEd25519PublicKey, serverEd25519PublicKeyErr := clientConf.ServerEd25519PublicKey()
	if serverEd25519PublicKeyErr != nil {
		return serverEd25519PublicKeyErr
	}

	if !clientCrypto.Verify(serverEd25519PublicKey, append(append(serverHello.CurvePublicKey, serverHello.Nonce...), sessionSalt...), serverHello.Signature) {
		return fmt.Errorf("server failed signature check")
	}

	dataToSign := append(append(sessionPublicKey, sessionSalt...), serverHello.Nonce...)
	signature := clientCrypto.Sign(edPrivateKey, dataToSign)
	clientIO.WriteClientSignature(signature)

	serverToClientKey, clientToServerKey, derivedSessionId, calculateKeysErr := clientCrypto.CalculateKeys(sessionPrivateKey[:], sessionSalt, serverHello.Nonce, serverHello.CurvePublicKey)
	if calculateKeysErr != nil {
		return calculateKeysErr
	}

	h.id = derivedSessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return nil
}
