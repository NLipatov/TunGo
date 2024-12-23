package handshake

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/hkdf"
	"io"
	"log"
	"net"
	"time"
	"tungo/Application/crypto/chacha20"
	"tungo/Domain/settings"
	"tungo/Domain/settings/client"
)

func OnConnectedToServer(ctx context.Context, conn net.Conn, settings settings.ConnectionSettings) (*chacha20.Session, error) {
	edPub, ed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 key pair: %s", err)
	}

	var curvePrivate [32]byte
	_, _ = io.ReadFull(rand.Reader, curvePrivate[:])
	curvePublic, _ := curve25519.X25519(curvePrivate[:], curve25519.Basepoint)
	nonce := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, nonce)

	rm, err := (&chacha20.ClientHello{}).Write(4, settings.InterfaceAddress, edPub, &curvePublic, &nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize registration message")
	}

	_, err = writeWithContext(ctx, conn, *rm)
	if err != nil {
		return nil, fmt.Errorf("failed to notice server on local address: %v", err)
	}

	//Read server hello
	sHBuf := make([]byte, 128)
	_, err = readWithContext(ctx, conn, sHBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read server-hello message")
	}

	serverHello, err := (&chacha20.ServerHello{}).Read(sHBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read server-hello message")
	}

	clientConf, err := (&client.Conf{}).Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read client configuration: %s", err)
	}
	serverEdPub := clientConf.Ed25519PublicKey
	if !ed25519.Verify(serverEdPub, append(append(serverHello.CurvePublicKey, serverHello.Nonce...), nonce...), serverHello.Signature) {
		return nil, fmt.Errorf("server failed signature check")
	}

	clientDataToSign := append(append(curvePublic, nonce...), serverHello.Nonce...)
	clientSignature := ed25519.Sign(ed, clientDataToSign)
	cS, err := (&chacha20.ClientSignature{}).Write(&clientSignature)
	if err != nil {
		return nil, fmt.Errorf("failed to create client signature message: %s", err)
	}

	_, err = writeWithContext(ctx, conn, *cS)
	if err != nil {
		return nil, fmt.Errorf("failed to send client signature message: %s", err)
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

	clientSession, err := chacha20.NewSession(clientToServerKey, serverToClientKey, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create client session: %s\n", err)
	}

	clientSession = clientSession.UseNonceRingBuffer(clientConf.UDPNonceRingBufferSize)

	derivedSessionId, deriveSessionIdErr := chacha20.DeriveSessionId(sharedSecret, salt[:])
	if deriveSessionIdErr != nil {
		return nil, fmt.Errorf("failed to derive session id: %s", derivedSessionId)
	}

	clientSession.SessionId = derivedSessionId

	return clientSession, nil
}

func readWithContext(ctx context.Context, conn net.Conn, buf []byte) (int, error) {
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

func writeWithContext(ctx context.Context, conn net.Conn, data []byte) (int, error) {
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
