package handshake

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"tungo/application"
	"tungo/settings"
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
	var clientHello ClientHello
	unmarshalErr := clientHello.UnmarshalBinary(buf)
	if unmarshalErr != nil {
		return nil, unmarshalErr
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

	serverHello, serverHelloErr := NewServerHello(serverSignature, serverNonce, curvePublic)
	if serverHelloErr != nil {
		return nil, fmt.Errorf("failed to create server hello: %s", serverHelloErr)
	}

	serverHelloBytes, serverHelloBytesErr := serverHello.MarshalBinary()
	if serverHelloBytesErr != nil {
		return nil, fmt.Errorf("failed to marshal server hello: %s", serverHelloBytesErr)
	}

	// Send server hello
	_, err = conn.Write(serverHelloBytes)
	clientSignatureBuf := make([]byte, 64)

	// Read client signature
	_, err = conn.Read(clientSignatureBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read client signature: %s\n", err)
	}

	var clientSignature ClientSignature
	unmarshalErr = clientSignature.UnmarshalBinary(clientSignatureBuf)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}

	// Verify client signature
	if !ed25519.Verify(clientHello.Ed25519PubKey, append(append(clientHello.CurvePubKey, clientHello.ClientNonce...), serverNonce...), clientSignature.Signature) {
		return nil, fmt.Errorf("client signature verification failed")
	}

	// Generate shared secret and salt
	sharedSecret, _ := curve25519.X25519(curvePrivate[:], clientHello.CurvePubKey)
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
