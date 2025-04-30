package handshake

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"testing"
	"tungo/settings"
)

// failWriteConn simulates write failures
type failWriteConn struct{ net.Conn }

func (f *failWriteConn) Write(b []byte) (int, error) { return 0, fmt.Errorf("write failed") }

// failReadConn simulates read failures
type failReadConn struct{ net.Conn }

func (f *failReadConn) Read(b []byte) (int, error) { return 0, fmt.Errorf("read failed") }

func TestSendClientHello_Success(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	session := make([]byte, 32)
	rand.Read(session)
	salt := make([]byte, 32)
	rand.Read(salt)
	cfg := settings.ConnectionSettings{InterfaceAddress: "192.168.0.1"}
	io := NewDefaultClientIO(client, cfg, pub, session, salt)

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 512)
		n, err := server.Read(buf)
		if err != nil || n == 0 {
			t.Errorf("server read error: %v, n=%d", err, n)
		}
		close(done)
	}()

	if err := io.SendClientHello(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	<-done
}

func TestSendClientHello_InvalidVersion(t *testing.T) {
	client, _ := net.Pipe()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	session := make([]byte, 32)
	rand.Read(session)
	salt := make([]byte, 32)
	rand.Read(salt)
	// create invalid ClientHello via settings
	cfg := settings.ConnectionSettings{InterfaceAddress: "x"}
	// use invalid version by passing through Write error of NewClientHello
	io := NewDefaultClientIO(client, cfg, pub, session, salt)
	err := io.SendClientHello()
	if err == nil {
		t.Error("expected error for invalid ip address, got nil")
	}
}

func TestSendClientHello_WriteError(t *testing.T) {
	client, _ := net.Pipe()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	session := make([]byte, 32)
	rand.Read(session)
	salt := make([]byte, 32)
	rand.Read(salt)
	cfg := settings.ConnectionSettings{InterfaceAddress: "10.0.0.1"}
	io := NewDefaultClientIO(&failWriteConn{client}, cfg, pub, session, salt)
	err := io.SendClientHello()
	if err == nil || !strings.Contains(err.Error(), "failed to write client hello") {
		t.Errorf("expected write error, got %v", err)
	}
}

func TestReceiveServerHello_Success(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// prepare a valid ServerHello payload
	sig := make([]byte, signatureLength)
	rand.Read(sig)
	nonce := make([]byte, nonceLength)
	rand.Read(nonce)
	curve := make([]byte, curvePublicKeyLength)
	rand.Read(curve)
	sh := ServerHello{sig, nonce, curve}
	data, _ := sh.MarshalBinary()

	io := NewDefaultClientIO(client, settings.ConnectionSettings{}, nil, nil, nil)
	go server.Write(data)

	out, err := io.ReceiveServerHello()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(out.Signature, sig) {
		t.Error("signature mismatch")
	}
}

func TestReceiveServerHello_ReadError(t *testing.T) {
	client, _ := net.Pipe()
	io := NewDefaultClientIO(&failReadConn{client}, settings.ConnectionSettings{}, nil, nil, nil)
	_, err := io.ReceiveServerHello()
	if err == nil || !strings.Contains(err.Error(), "failed to read server hello message") {
		t.Errorf("expected read error, got %v", err)
	}
}

func TestWriteClientSignature_Success(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	io := NewDefaultClientIO(client, settings.ConnectionSettings{}, nil, nil, nil)
	sig := make([]byte, signatureLength)
	rand.Read(sig)
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 512)
		n, _ := server.Read(buf)
		if n == 0 {
			t.Error("expected bytes")
		}
		close(done)
	}()
	if err := io.WriteClientSignature(sig); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	<-done
}

func TestWriteClientSignature_SerializeError(t *testing.T) {
	client, _ := net.Pipe()
	io := NewDefaultClientIO(client, settings.ConnectionSettings{}, nil, nil, nil)
	// too-short signature
	sig := []byte{1, 2, 3}
	err := io.WriteClientSignature(sig)
	if err == nil || !strings.Contains(err.Error(), "handshake: cannot create ClientSignature: handshake: invalid client signature length") {
		t.Errorf("expected serialize error, got %v", err)
	}
}

func TestWriteClientSignature_WriteError(t *testing.T) {
	client, _ := net.Pipe()
	io := NewDefaultClientIO(&failWriteConn{client}, settings.ConnectionSettings{}, nil, nil, nil)
	sig := make([]byte, signatureLength)
	rand.Read(sig)
	err := io.WriteClientSignature(sig)
	if err == nil || !strings.Contains(err.Error(), "failed to send client signature message") {
		t.Errorf("expected send error, got %v", err)
	}
}
