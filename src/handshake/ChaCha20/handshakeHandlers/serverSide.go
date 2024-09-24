package handshakeHandlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"etha-tunnel/handshake/ChaCha20"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
	"io"
	"log"
	"net"
)

func OnClientConnected(conn net.Conn) (*ChaCha20.Session, *string, error) {
	/*edPublic*/ _, edPrivate := "m+tjQmYAG8tYt8xSTry29Mrl9SInd9pvoIsSywzPzdU=", "ZuQO8SI3rxY/v1sJn9DtGQ2vRgz/DiPg545iFYmSWleb62NCZgAby1i3zFJOvLb0yuX1Iid32m+gixLLDM/N1Q=="

	buf := make([]byte, 39+2+32+32+32) // 39(max ip) + 2(length headers) + 32 (ed25519 pub key) + 32 (curve pub key)
	_, err := conn.Read(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read from client: %v\n", err)
	}

	//Read client hello
	clientHello, err := (&ChaCha20.ClientHello{}).Read(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to deserialize registration message: %s\n", err)
	}

	// Generate server hello response
	var curvePrivate [32]byte
	_, _ = io.ReadFull(rand.Reader, curvePrivate[:])
	curvePublic, _ := curve25519.X25519(curvePrivate[:], curve25519.Basepoint)
	serverNonce := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, serverNonce)
	serverDataToSign := append(append(curvePublic, serverNonce...), clientHello.ClientNonce...)
	privateEd, err := base64.StdEncoding.DecodeString(edPrivate)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode private ed key: %s\n", err)
	}
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
