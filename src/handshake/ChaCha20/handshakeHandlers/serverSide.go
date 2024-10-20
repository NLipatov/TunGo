package handshakeHandlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/settings/server"
	"fmt"
	"io"
	"log"
	"net"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

func OnClientConnected(conn net.Conn) (*ChaCha20.Session, *string, error) {
	conf, err := (&server.Conf{}).Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read server conf: %s", err)
	}

	buf := make([]byte, ChaCha20.MaxClientHelloSizeBytes)
	_, err = conn.Read(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read from client: %v", err)
	}

	//Read client hello
	clientHello, err := (&ChaCha20.ClientHello{}).Read(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid client hello: %s", err)
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
	serverHello, err := (&ChaCha20.ServerHello{}).Write(&serverSignature, &serverNonce, &curvePublic)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write server hello: %s\n", err)
	}

	// Send server hello
	_, err = conn.Write(*serverHello)
	clientSignatureBuf := make([]byte, 64)

	// Read client signature
	_, err = conn.Read(clientSignatureBuf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read client signature: %s\n", err)
	}
	clientSignature, err := (&ChaCha20.ClientSignature{}).Read(clientSignatureBuf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read client signature: %s", err)
	}

	// Verify client signature
	if !ed25519.Verify(clientHello.EdPublicKey, append(append(clientHello.CurvePublicKey, clientHello.ClientNonce...), serverNonce...), clientSignature.ClientSignature) {
		return nil, nil, fmt.Errorf("client signature verification failed: %s\n", err)
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

	// Generate server session
	serverSession, err := ChaCha20.NewSession(serverToClientKey, clientToServerKey, true)
	if err != nil {
		log.Fatalf("failed to create server session: %s\n", err)
	}

	serverSession.SessionId = sha256.Sum256(append(sharedSecret, salt[:]...))

	return serverSession, &clientHello.IpAddress, nil
}
