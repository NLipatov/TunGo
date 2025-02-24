package chacha20

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
	"tungo/network"
	"tungo/settings"
	"tungo/settings/client"
	"tungo/settings/server"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/hkdf"
)

type Handshake interface {
	Id() [32]byte
	ClientKey() []byte
	ServerKey() []byte
	ServerSideHandshake(conn network.ConnectionAdapter) (*string, error)
	ClientSideHandshake(ctx context.Context, conn net.Conn, settings settings.ConnectionSettings) error
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

func (h *HandshakeImpl) ServerSideHandshake(conn network.ConnectionAdapter) (*string, error) {
	conf, err := (&server.Conf{}).Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read server conf: %s", err)
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

	derivedSessionId, deriveSessionIdErr := DeriveSessionId(sharedSecret, salt[:])
	if deriveSessionIdErr != nil {
		return nil, fmt.Errorf("failed to derive session id: %s", derivedSessionId)
	}

	h.id = derivedSessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return &clientHello.IpAddress, nil
}

func (h *HandshakeImpl) ClientSideHandshake(ctx context.Context, conn net.Conn, settings settings.ConnectionSettings) error {
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

	_, err = h.writeWithContext(ctx, conn, *rm)
	if err != nil {
		return fmt.Errorf("failed to notice server on local address: %v", err)
	}

	//Read server hello
	sHBuf := make([]byte, 128)
	_, err = h.readWithContext(ctx, conn, sHBuf)
	if err != nil {
		return fmt.Errorf("failed to read server-hello message")
	}

	serverHello, err := (&ServerHello{}).Read(sHBuf)
	if err != nil {
		return fmt.Errorf("failed to read server-hello message")
	}

	clientConf, err := (&client.Conf{}).Read()
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

	_, err = h.writeWithContext(ctx, conn, *cS)
	if err != nil {
		return fmt.Errorf("failed to send client signature message: %s", err)
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

	derivedSessionId, deriveSessionIdErr := DeriveSessionId(sharedSecret, salt[:])
	if deriveSessionIdErr != nil {
		return fmt.Errorf("failed to derive session id: %s", derivedSessionId)
	}

	h.id = derivedSessionId
	h.clientKey = clientToServerKey
	h.serverKey = serverToClientKey

	return nil
}

func (h *HandshakeImpl) readWithContext(ctx context.Context, conn net.Conn, buf []byte) (int, error) {
	select {
	case <-ctx.Done(): //if ctx already cancelled
		return 0, fmt.Errorf("operation canceled before reading: %w", ctx.Err())
	default:
	}

	deadline, ok := ctx.Deadline()
	if ok {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return 0, fmt.Errorf("failed to set read deadline: %w", err)
		}
	}

	n, err := conn.Read(buf)

	if ok {
		if resetErr := conn.SetReadDeadline(time.Time{}); resetErr != nil {
			log.Printf("failed to reset read deadline: %v", resetErr)
		}
	}

	if err != nil {
		if errors.Is(err, io.EOF) {
			return 0, fmt.Errorf("connection closed by peer: %w", err)
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return 0, fmt.Errorf("read timed out: %w", err)
		}
		return 0, fmt.Errorf("unexpected read error: %w", err)
	}

	return n, nil
}

func (h *HandshakeImpl) writeWithContext(ctx context.Context, conn net.Conn, data []byte) (int, error) {
	select {
	case <-ctx.Done(): //if ctx already cancelled
		return 0, fmt.Errorf("operation canceled before writing: %w", ctx.Err())
	default:
	}

	deadline, ok := ctx.Deadline()
	if ok {
		if err := conn.SetWriteDeadline(deadline); err != nil {
			return 0, fmt.Errorf("failed to set write deadline: %w", err)
		}
	}

	n, err := conn.Write(data)

	if ok {
		if resetErr := conn.SetWriteDeadline(time.Time{}); resetErr != nil {
			log.Printf("failed to reset write deadline: %v", resetErr)
		}
	}

	if err != nil {
		if errors.Is(err, io.EOF) {
			return 0, fmt.Errorf("connection closed by peer: %w", err)
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return 0, fmt.Errorf("write timed out: %w", err)
		}
		return 0, fmt.Errorf("unexpected write error: %w", err)
	}

	return n, nil
}
