package chacha20

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
	edPub, ed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate ed25519 key pair: %s", err)
	}

	var curvePrivate [32]byte
	_, _ = io.ReadFull(rand.Reader, curvePrivate[:])
	curvePublic, _ := curve25519.X25519(curvePrivate[:], curve25519.Basepoint)
	nonce := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, nonce)

	rm, err := (&ClientHello{}).Write(4, settings.InterfaceAddress, edPub, &curvePublic, &nonce)
	if err != nil {
		return fmt.Errorf("failed to serialize registration message")
	}

	_, rmWriteErr := conn.Write(*rm)
	if rmWriteErr != nil {
		return fmt.Errorf("failed to write registration message: %s", rmWriteErr)
	}

	//Read server hello
	shmBuf := make([]byte, 128)
	_, shmErr := conn.Read(shmBuf)
	if shmErr != nil {
		return fmt.Errorf("failed to read server-hello message")
	}

	serverHello, err := (&ServerHello{}).Read(shmBuf)
	if err != nil {
		return fmt.Errorf("failed to read server-hello message")
	}

	configurationManager := client_configuration.NewManager()
	clientConf, err := configurationManager.Configuration()
	if err != nil {
		return fmt.Errorf("failed to read client configuration: %s", err)
	}
	serverEdPub := clientConf.Ed25519PublicKey
	if !ed25519.Verify(serverEdPub, append(append(serverHello.CurvePublicKey, serverHello.Nonce...), nonce...), serverHello.Signature) {
		return fmt.Errorf("server failed signature check")
	}

	clientDataToSign := append(append(curvePublic, nonce...), serverHello.Nonce...)
	clientSignature := ed25519.Sign(ed, clientDataToSign)
	cS, err := (&ClientSignature{}).Write(&clientSignature)
	if err != nil {
		return fmt.Errorf("failed to create client signature message: %s", err)
	}

	_, csErr := conn.Write(*cS)
	if csErr != nil {
		return fmt.Errorf("failed to send client signature message: %s", csErr)
	}

	sharedSecret, _ := curve25519.X25519(curvePrivate[:], serverHello.CurvePublicKey)
	salt := sha256.Sum256(append(serverHello.Nonce, nonce...))
	infoSC := []byte("server-to-client") // server-key info
	infoCS := []byte("client-to-server") // client-key info
	serverToClientHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoSC)
	clientToServerHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoCS)
	keySize := chacha20poly1305.KeySize
	serverToClientKey := make([]byte, keySize)
	_, _ = io.ReadFull(serverToClientHKDF, serverToClientKey)
	clientToServerKey := make([]byte, keySize)
	_, _ = io.ReadFull(clientToServerHKDF, clientToServerKey)

	derivedSessionId, deriveSessionIdErr := deriveSessionId(sharedSecret, salt[:])
	if deriveSessionIdErr != nil {
		return fmt.Errorf("failed to derive session id: %s", derivedSessionId)
	}

	h.id = derivedSessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return nil
}

func deriveSessionId(sharedSecret []byte, salt []byte) ([32]byte, error) {
	var sessionID [32]byte

	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt, []byte("session-id-derivation"))
	if _, err := io.ReadFull(hkdfReader, sessionID[:]); err != nil {
		return [32]byte{}, fmt.Errorf("failed to derive session ID: %w", err)
	}

	return sessionID, nil
}
