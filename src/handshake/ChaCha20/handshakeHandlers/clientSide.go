package handshakeHandlers

import (
	"crypto/rand"
	"crypto/sha256"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/settings/client"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/hkdf"
	"io"
	"net"
)

func OnConnectedToServer(conn net.Conn, conf *client.Conf) (*ChaCha20.Session, error) {
	edPub, ed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 key pair: %s", err)
	}

	var curvePrivate [32]byte
	_, _ = io.ReadFull(rand.Reader, curvePrivate[:])
	curvePublic, _ := curve25519.X25519(curvePrivate[:], curve25519.Basepoint)
	nonce := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, nonce)

	rm, err := (&ChaCha20.ClientHello{}).Write(4, conf.TCPSettings.InterfaceAddress, edPub, &curvePublic, &nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize registration message")
	}

	_, err = conn.Write(*rm)
	if err != nil {
		return nil, fmt.Errorf("failed to notice server on local address: %v", err)
	}

	//Mocked server hello
	sHBuf := make([]byte, 128)
	_, err = conn.Read(sHBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read server-hello message")
	}

	serverHello, err := (&ChaCha20.ServerHello{}).Read(sHBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read server-hello message")
	}

	clientConf, err := (&client.Conf{}).Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read client configuration: %s", err)
	}
	serverEdPub := clientConf.Ed25519PublicKey
	if !ed25519.Verify(serverEdPub, append(append(serverHello.CurvePublicKey, serverHello.ServerNonce...), nonce...), serverHello.ServerSignature) {
		return nil, fmt.Errorf("server failed signature check")
	}

	clientDataToSign := append(append(curvePublic, nonce...), serverHello.ServerNonce...)
	clientSignature := ed25519.Sign(ed, clientDataToSign)
	cS, err := (&ChaCha20.ClientSignature{}).Write(&clientSignature)
	if err != nil {
		return nil, fmt.Errorf("failed to create client signature message: %s", err)
	}

	_, err = conn.Write(*cS)
	if err != nil {
		return nil, fmt.Errorf("failed to send client signature message: %s", err)
	}

	sharedSecret, _ := curve25519.X25519(curvePrivate[:], serverHello.CurvePublicKey)
	salt := sha256.Sum256(append(serverHello.ServerNonce, nonce...))
	infoSC := []byte("server-to-client") // server-key info
	infoCS := []byte("client-to-server") // client-key info
	serverToClientHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoSC)
	clientToServerHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoCS)
	keySize := chacha20poly1305.KeySize
	serverToClientKey := make([]byte, keySize)
	_, _ = io.ReadFull(serverToClientHKDF, serverToClientKey)
	clientToServerKey := make([]byte, keySize)
	_, _ = io.ReadFull(clientToServerHKDF, clientToServerKey)

	clientSession, err := ChaCha20.NewSession(clientToServerKey, serverToClientKey, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create client session: %s\n", err)
	}

	clientSession.SessionId = sha256.Sum256(append(sharedSecret, salt[:]...))

	return clientSession, nil
}
