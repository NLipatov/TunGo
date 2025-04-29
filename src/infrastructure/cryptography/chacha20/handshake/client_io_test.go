package handshake

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"tungo/settings"
)

// ClientIOTestFailingWriteConn simulates a Write error.
type ClientIOTestFailingWriteConn struct{ net.Conn }

func (c *ClientIOTestFailingWriteConn) Write(b []byte) (int, error) {
	return 0, fmt.Errorf("write failure")
}

// ClientIOTestFailingReadConn simulates a Read error.
type ClientIOTestFailingReadConn struct{ net.Conn }

func (c *ClientIOTestFailingReadConn) Read(b []byte) (int, error) {
	return 0, fmt.Errorf("read failure")
}

func TestWriteClientHello_Success(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sessionPub := make([]byte, 32)
	rand.Read(sessionPub)
	salt := make([]byte, 32)
	rand.Read(salt)
	cfg := settings.ConnectionSettings{InterfaceAddress: "10.0.0.5"}

	io := NewDefaultClientIO(clientConn, cfg, pub, sessionPub, salt)

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 512)
		n, err := serverConn.Read(buf)
		if err != nil {
			t.Errorf("server read error: %v", err)
		}
		if n == 0 {
			t.Errorf("expected some bytes, got 0")
		}
		close(done)
	}()
	if err := io.WriteClientHello(); err != nil {
		t.Fatalf("WriteClientHello failed: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for server read")
	}
}

func TestWriteClientHello_WriteError(t *testing.T) {
	clientConn, _ := net.Pipe()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sessionPub := make([]byte, 32)
	rand.Read(sessionPub)
	salt := make([]byte, 32)
	rand.Read(salt)
	cfg := settings.ConnectionSettings{InterfaceAddress: "10.0.0.1"}
	bad := &ClientIOTestFailingWriteConn{clientConn}
	io := NewDefaultClientIO(bad, cfg, pub, sessionPub, salt)

	err := io.WriteClientHello()
	if err == nil || !strings.Contains(err.Error(), "failed to write client hello") {
		t.Errorf("expected write error, got %v", err)
	}
}

func TestReadServerHello_Success(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	sig := make([]byte, 64)
	rand.Read(sig)
	nonce := make([]byte, 32)
	rand.Read(nonce)
	curvePub := make([]byte, 32)
	rand.Read(curvePub)
	shBytes, _ := (&ServerHello{}).Write(&sig, &nonce, &curvePub)

	io := NewDefaultClientIO(clientConn, settings.ConnectionSettings{}, nil, nil, nil)

	go serverConn.Write(*shBytes)

	sh, err := io.ReadServerHello()
	if err != nil {
		t.Fatalf("ReadServerHello failed: %v", err)
	}
	if !bytes.Equal(sh.Signature, sig) {
		t.Errorf("signature mismatch")
	}
	if !bytes.Equal(sh.Nonce, nonce) {
		t.Errorf("nonce mismatch")
	}
	if !bytes.Equal(sh.CurvePublicKey, curvePub) {
		t.Errorf("curvePub mismatch")
	}
}

func TestReadServerHello_ReadError(t *testing.T) {
	clientConn, _ := net.Pipe()
	bad := &ClientIOTestFailingReadConn{clientConn}
	io := NewDefaultClientIO(bad, settings.ConnectionSettings{}, nil, nil, nil)

	_, err := io.ReadServerHello()
	if err == nil || !strings.Contains(err.Error(), "failed to read server hello message") {
		t.Errorf("expected read error, got %v", err)
	}
}

func TestWriteClientSignature_Success(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	io := NewDefaultClientIO(clientConn, settings.ConnectionSettings{}, nil, nil, nil)
	sig := make([]byte, 64)
	rand.Read(sig)

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 128)
		n, err := serverConn.Read(buf)
		if err != nil {
			t.Errorf("server read error: %v", err)
		}
		if n == 0 {
			t.Errorf("expected some bytes, got 0")
		}
		close(done)
	}()
	if err := io.WriteClientSignature(sig); err != nil {
		t.Fatalf("WriteClientSignature failed: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for server read")
	}
}

func TestWriteClientSignature_SerializeError(t *testing.T) {
	clientConn, _ := net.Pipe()
	io := NewDefaultClientIO(clientConn, settings.ConnectionSettings{}, nil, nil, nil)
	sig := []byte{1, 2, 3}
	err := io.WriteClientSignature(sig)
	if err == nil || !strings.Contains(err.Error(), "failed to create client signature message") {
		t.Errorf("expected serialize error, got %v", err)
	}
}
