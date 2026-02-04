package noise

import (
	"bytes"
	"net"
	"strings"
	"testing"
	framelimit "tungo/domain/network/ip/frame_limit"
	"tungo/infrastructure/network/tcp/adapters"
	"tungo/infrastructure/settings"
)

func TestZeroBytes(t *testing.T) {
	b := []byte{1, 2, 3, 4, 5}
	ZeroBytes(b)
	for i, v := range b {
		if v != 0 {
			t.Fatalf("byte %d not zeroed: %d", i, v)
		}
	}
}

func TestZeroBytes_Empty(t *testing.T) {
	b := []byte{}
	ZeroBytes(b) // should not panic
}

func TestNewNoiseHandshake(t *testing.T) {
	pub := make([]byte, 32)
	priv := make([]byte, 32)
	pub[0] = 1
	priv[0] = 2

	h := NewNoiseHandshake(pub, priv)
	if h == nil {
		t.Fatal("NewNoiseHandshake returned nil")
	}
	if len(h.staticPublicKey) != 32 {
		t.Fatalf("wrong public key length: %d", len(h.staticPublicKey))
	}
	if len(h.staticPrivateKey) != 32 {
		t.Fatalf("wrong private key length: %d", len(h.staticPrivateKey))
	}
}

func TestNewNoiseHandshake_ClientOnly(t *testing.T) {
	pub := make([]byte, 32)
	h := NewNoiseHandshake(pub, nil)
	if h.staticPrivateKey != nil {
		t.Fatal("client should have nil private key")
	}
}

func TestNoiseHandshake_InitialState(t *testing.T) {
	h := NewNoiseHandshake(make([]byte, 32), make([]byte, 32))

	var zeroID [32]byte
	if h.Id() != zeroID {
		t.Fatal("initial ID should be zero")
	}
	if h.KeyClientToServer() != nil {
		t.Fatal("initial c2s key should be nil")
	}
	if h.KeyServerToClient() != nil {
		t.Fatal("initial s2c key should be nil")
	}
}

func TestNoiseXX_EndToEnd(t *testing.T) {
	// Generate server static X25519 keypair.
	serverKP, err := cipherSuite.GenerateKeypair(nil)
	if err != nil {
		t.Fatalf("generate server keypair: %v", err)
	}

	// Connected pair with length-prefix framing.
	clientConn, serverConn := net.Pipe()
	defer func(clientConn net.Conn) {
		_ = clientConn.Close()
	}(clientConn)
	defer func(serverConn net.Conn) {
		_ = serverConn.Close()
	}(serverConn)

	clientAdapter, err := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(512))
	if err != nil {
		t.Fatalf("client adapter: %v", err)
	}
	serverAdapter, err := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(512))
	if err != nil {
		t.Fatalf("server adapter: %v", err)
	}

	serverHS := NewNoiseHandshake(serverKP.Public, serverKP.Private)
	clientHS := NewNoiseHandshake(serverKP.Public, nil)

	// Run both sides concurrently.
	var srvIP net.IP

	srvCh := make(chan error, 1)
	cliCh := make(chan error, 1)

	go func() {
		ip, err := serverHS.ServerSideHandshake(serverAdapter)
		srvIP = ip
		srvCh <- err
	}()
	go func() {
		cliCh <- clientHS.ClientSideHandshake(
			clientAdapter, settings.Settings{InterfaceAddress: "10.0.0.2"},
		)
	}()

	// Handshake completes without error on both sides.
	if err := <-srvCh; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
	if err := <-cliCh; err != nil {
		t.Fatalf("client handshake: %v", err)
	}

	// Keys match directionally.
	if !bytes.Equal(serverHS.clientKey, clientHS.clientKey) {
		t.Fatal("client-to-server session key mismatch between sides")
	}
	if !bytes.Equal(serverHS.serverKey, clientHS.serverKey) {
		t.Fatal("server-to-client session key mismatch between sides")
	}

	// Session IDs match and are non-zero.
	if serverHS.id != clientHS.id {
		t.Fatal("session ID mismatch")
	}
	if serverHS.id == ([32]byte{}) {
		t.Fatal("session ID is zero")
	}

	// Client IP received correctly.
	expectedIP := net.ParseIP("10.0.0.2").To4()
	if !srvIP.Equal(expectedIP) {
		t.Fatalf("server got client IP %v, want %v", srvIP, expectedIP)
	}
}

func TestNoiseXX_ServerAuthRejection(t *testing.T) {
	// Generate two independent server keypairs. Client expects one, server uses the other.
	realServerKP, err := cipherSuite.GenerateKeypair(nil)
	if err != nil {
		t.Fatalf("generate real server keypair: %v", err)
	}
	impostorKP, err := cipherSuite.GenerateKeypair(nil)
	if err != nil {
		t.Fatalf("generate impostor keypair: %v", err)
	}

	clientConn, serverConn := net.Pipe()
	defer func(clientConn net.Conn) {
		_ = clientConn.Close()
	}(clientConn)
	defer func(serverConn net.Conn) {
		_ = serverConn.Close()
	}(serverConn)

	clientAdapter, err := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(512))
	if err != nil {
		t.Fatalf("client adapter: %v", err)
	}
	serverAdapter, err := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(512))
	if err != nil {
		t.Fatalf("server adapter: %v", err)
	}

	// Server uses impostor keys; client expects real server's public key.
	serverHS := NewNoiseHandshake(impostorKP.Public, impostorKP.Private)
	clientHS := NewNoiseHandshake(realServerKP.Public, nil)

	srvCh := make(chan error, 1)
	cliCh := make(chan error, 1)

	go func() {
		_, err := serverHS.ServerSideHandshake(serverAdapter)
		srvCh <- err
	}()
	go func() {
		cliCh <- clientHS.ClientSideHandshake(
			clientAdapter, settings.Settings{InterfaceAddress: "10.0.0.2"},
		)
	}()

	cliErr := <-cliCh
	if cliErr == nil {
		t.Fatal("client should reject impostor server, got nil error")
	}
	if !strings.Contains(cliErr.Error(), "server static key mismatch") {
		t.Fatalf("expected key mismatch error, got: %v", cliErr)
	}
}
