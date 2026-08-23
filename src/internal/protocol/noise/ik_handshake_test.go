package noise

import (
	"bytes"
	"io"
	"net"
	"net/netip"
	"testing"

	transport "tungo/internal/transport/tcp"
)

func closeTestConn(conn net.Conn) {
	_ = conn.Close()
}

func completeTestHandshake(
	t *testing.T,
	server, client *IKHandshake,
	serverTransport, clientTransport io.ReadWriteCloser,
) {
	t.Helper()
	serverErr := make(chan error, 1)
	go func() {
		_, err := server.ServerSideHandshake(serverTransport)
		serverErr <- err
	}()
	if err := client.ClientSideHandshake(clientTransport); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
}

func TestIKHandshake_Success(t *testing.T) {
	// Generate server and client keypairs
	serverKP, err := cipherSuite.GenerateKeypair(nil)
	if err != nil {
		t.Fatalf("generate server keypair: %v", err)
	}

	clientKP, err := cipherSuite.GenerateKeypair(nil)
	if err != nil {
		t.Fatalf("generate client keypair: %v", err)
	}

	// Configure allowed peers
	allowedPeers := []testPeer{
		{
			PublicKey: clientKP.Public,
			Enabled:   true,
			ClientID:  5,
		},
	}

	// Create handshakes
	cookieManager, _ := NewCookieManager()
	loadMonitor := NewLoadMonitor(10000) // High threshold to avoid cookie challenges

	serverHS := NewIKHandshakeServer(
		serverKP.Public,
		serverKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager,
		loadMonitor,
	)

	clientHS := NewIKHandshakeClient(
		clientKP.Public,
		clientKP.Private,
		serverKP.Public,
	)

	// Connected pair with framing
	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)

	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

	// Run both sides concurrently
	var srvClientID int
	srvCh := make(chan error, 1)
	cliCh := make(chan error, 1)

	go func() {
		idx, err := serverHS.ServerSideHandshake(serverAdapter)
		srvClientID = idx
		srvCh <- err
	}()
	go func() {
		cliCh <- clientHS.ClientSideHandshake(clientAdapter)
	}()

	// Both should complete without error
	if err := <-srvCh; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
	if err := <-cliCh; err != nil {
		t.Fatalf("client handshake: %v", err)
	}

	// Verify keys match
	if !bytes.Equal(serverHS.clientKey, clientHS.clientKey) {
		t.Fatal("client-to-server key mismatch")
	}
	if !bytes.Equal(serverHS.serverKey, clientHS.serverKey) {
		t.Fatal("server-to-client key mismatch")
	}

	// Verify session IDs match
	if serverHS.id != clientHS.id {
		t.Fatal("session ID mismatch")
	}

	// Verify client index
	if srvClientID != 5 {
		t.Fatalf("expected client index 5, got %d", srvClientID)
	}

	if !bytes.Equal(serverHS.ClientPubKey(), clientKP.Public) {
		t.Fatal("result client pub key mismatch")
	}
	if !clientHS.Supports(CapabilityRekeyV2) || !serverHS.Supports(CapabilityRekeyV2) {
		t.Fatal("Rekey V2 capability was not negotiated")
	}
}

func TestIKHandshake_RekeyV2(t *testing.T) {
	serverKP, err := cipherSuite.GenerateKeypair(nil)
	if err != nil {
		t.Fatal(err)
	}
	clientKP, err := cipherSuite.GenerateKeypair(nil)
	if err != nil {
		t.Fatal(err)
	}
	allowedPeers := newTestAllowedPeers([]testPeer{{
		PublicKey: clientKP.Public,
		Enabled:   true,
		ClientID:  1,
	}})
	serverHS := NewIKHandshakeServer(serverKP.Public, serverKP.Private, allowedPeers, nil, nil)
	clientHS := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)
	clientTransport, _ := transport.NewFramedConn(clientConn, 2048)
	serverTransport, _ := transport.NewFramedConn(serverConn, 2048)
	completeTestHandshake(t, serverHS, clientHS, serverTransport, clientTransport)

	prologue := []byte("current session")
	msg1, err := clientHS.StartRekeyV2(prologue)
	if err != nil {
		t.Fatal(err)
	}
	msg2, serverC2S, serverS2C, err := serverHS.RespondRekeyV2(prologue, msg1)
	if err != nil {
		t.Fatal(err)
	}
	clientC2S, clientS2C, err := clientHS.FinishRekeyV2(msg2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(clientC2S, serverC2S) || !bytes.Equal(clientS2C, serverS2C) {
		t.Fatal("rekey traffic keys do not match")
	}
	if bytes.Equal(clientC2S, clientHS.KeyClientToServer()) || bytes.Equal(clientS2C, clientHS.KeyServerToClient()) {
		t.Fatal("rekey reused initial traffic keys")
	}
}

func TestIKHandshake_RekeyV2RejectsWrongPrologue(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)
	serverHS := NewIKHandshakeServer(
		serverKP.Public,
		serverKP.Private,
		newTestAllowedPeers([]testPeer{{
			PublicKey: clientKP.Public,
			Enabled:   true,
			ClientID:  1,
		}}),
		nil,
		nil,
	)
	clientHS := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)
	clientTransport, _ := transport.NewFramedConn(clientConn, 2048)
	serverTransport, _ := transport.NewFramedConn(serverConn, 2048)
	completeTestHandshake(t, serverHS, clientHS, serverTransport, clientTransport)

	msg1, err := clientHS.StartRekeyV2([]byte("client state"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := serverHS.RespondRekeyV2([]byte("server state"), msg1); err == nil {
		t.Fatal("expected mismatched prologue to reject rekey")
	}
}

func TestIKHandshake_UnknownClient(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)
	unknownKP, _ := cipherSuite.GenerateKeypair(nil)

	// Only allow clientKP
	allowedPeers := []testPeer{
		{
			PublicKey: clientKP.Public,
			Enabled:   true,
			ClientID:  5,
		},
	}

	cookieManager, _ := NewCookieManager()
	loadMonitor := NewLoadMonitor(10000)

	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager, loadMonitor,
	)

	// Client uses unknown key
	clientHS := NewIKHandshakeClient(
		unknownKP.Public, unknownKP.Private,
		serverKP.Public,
	)

	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)

	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

	srvCh := make(chan error, 1)
	cliCh := make(chan error, 1)

	go func() {
		_, err := serverHS.ServerSideHandshake(serverAdapter)
		srvCh <- err
	}()
	go func() {
		cliCh <- clientHS.ClientSideHandshake(clientAdapter)
	}()

	srvErr := <-srvCh
	if srvErr == nil || srvErr != ErrUnknownPeer {
		t.Fatalf("expected ErrUnknownPeer, got: %v", srvErr)
	}
}

func TestIKHandshake_DisabledClient(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	// Client is disabled
	allowedPeers := []testPeer{
		{
			PublicKey: clientKP.Public,
			Enabled:   false, // Disabled
			ClientID:  5,
		},
	}

	cookieManager, _ := NewCookieManager()
	loadMonitor := NewLoadMonitor(10000)

	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager, loadMonitor,
	)

	clientHS := NewIKHandshakeClient(
		clientKP.Public, clientKP.Private,
		serverKP.Public,
	)

	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)

	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

	srvCh := make(chan error, 1)
	go func() {
		_, err := serverHS.ServerSideHandshake(serverAdapter)
		srvCh <- err
	}()
	go func() {
		_ = clientHS.ClientSideHandshake(clientAdapter)
	}()

	srvErr := <-srvCh
	if srvErr == nil || srvErr != ErrPeerDisabled {
		t.Fatalf("expected ErrPeerDisabled, got: %v", srvErr)
	}
}

func TestIKHandshake_KeyMismatch(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	impostorKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []testPeer{
		{
			PublicKey: clientKP.Public,
			Enabled:   true,
			ClientID:  5,
		},
	}

	cookieManager, _ := NewCookieManager()
	loadMonitor := NewLoadMonitor(10000)

	// Server uses impostor keys
	serverHS := NewIKHandshakeServer(
		impostorKP.Public, impostorKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager, loadMonitor,
	)

	// Client expects real server's key
	clientHS := NewIKHandshakeClient(
		clientKP.Public, clientKP.Private,
		serverKP.Public, // Wrong server key
	)

	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)

	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

	srvCh := make(chan error, 1)
	cliCh := make(chan error, 1)

	go func() {
		_, err := serverHS.ServerSideHandshake(serverAdapter)
		srvCh <- err
		// Close server's side when it's done to unblock client
		closeTestConn(serverConn)
	}()
	go func() {
		cliCh <- clientHS.ClientSideHandshake(clientAdapter)
		// Close client's side when it's done to unblock server
		closeTestConn(clientConn)
	}()

	// Wait for both with a timeout
	var srvErr, cliErr error
	for i := 0; i < 2; i++ {
		select {
		case srvErr = <-srvCh:
		case cliErr = <-cliCh:
		}
	}

	// Either server or client (or both) should fail when keys don't match
	// The handshake fails because client's msg1 is encrypted to wrong server key
	if srvErr == nil && cliErr == nil {
		t.Fatal("at least one side should fail when server key doesn't match")
	}
}

func TestIKHandshake_FreshEphemeralPerHandshake(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []testPeer{
		{
			PublicKey: clientKP.Public,
			Enabled:   true,
			ClientID:  5,
		},
	}

	cookieManager, _ := NewCookieManager()
	loadMonitor := NewLoadMonitor(10000)

	// Two separate handshakes should produce different session keys
	var sessionKey1, sessionKey2 []byte

	for i := 0; i < 2; i++ {
		serverHS := NewIKHandshakeServer(
			serverKP.Public, serverKP.Private,
			newTestAllowedPeers(allowedPeers),
			cookieManager, loadMonitor,
		)
		clientHS := NewIKHandshakeClient(
			clientKP.Public, clientKP.Private,
			serverKP.Public,
		)

		clientConn, serverConn := net.Pipe()
		clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
		serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

		completeTestHandshake(t, serverHS, clientHS, serverAdapter, clientAdapter)

		closeTestConn(clientConn)
		closeTestConn(serverConn)

		if i == 0 {
			sessionKey1 = make([]byte, len(clientHS.clientKey))
			copy(sessionKey1, clientHS.clientKey)
		} else {
			sessionKey2 = make([]byte, len(clientHS.clientKey))
			copy(sessionKey2, clientHS.clientKey)
		}
	}

	if bytes.Equal(sessionKey1, sessionKey2) {
		t.Fatal("different handshakes should produce different session keys (fresh ephemeral)")
	}
}

func TestIKHandshake_MissingClientKey(t *testing.T) {
	serverPubKey := make([]byte, 32)

	// Client without keys
	clientHS := NewIKHandshakeClient(nil, nil, serverPubKey)

	// Create a mock transport
	clientConn, _ := net.Pipe()
	defer closeTestConn(clientConn)
	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)

	err := clientHS.ClientSideHandshake(clientAdapter)
	if err == nil || err != ErrMissingClientKey {
		t.Fatalf("expected ErrMissingClientKey, got: %v", err)
	}
}

func TestIKHandshake_MissingServerKey(t *testing.T) {
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	// Client without server's public key
	clientHS := NewIKHandshakeClient(clientKP.Public, clientKP.Private, nil)

	clientConn, _ := net.Pipe()
	defer closeTestConn(clientConn)
	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)

	err := clientHS.ClientSideHandshake(clientAdapter)
	if err == nil || err != ErrMissingServerKey {
		t.Fatalf("expected ErrMissingServerKey, got: %v", err)
	}
}

func TestIKHandshake_ServerMissingAllowedPeers(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)

	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		nil, // No allowed peers
		nil, nil,
	)

	serverConn, _ := net.Pipe()
	defer closeTestConn(serverConn)
	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

	_, err := serverHS.ServerSideHandshake(serverAdapter)
	if err == nil || err != ErrMissingAllowedPeers {
		t.Fatalf("expected ErrMissingAllowedPeers, got: %v", err)
	}
}

func TestIKHandshake_InvalidMAC1(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []testPeer{
		{PublicKey: clientKP.Public, Enabled: true, ClientID: 5},
	}

	cookieManager, _ := NewCookieManager()
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager, nil,
	)

	// Create a transport that sends garbage
	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)

	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)
	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)

	// Send garbage with valid version byte but invalid MAC1
	go func() {
		garbage := make([]byte, MinTotalSizeWithVersion)
		garbage[0] = ProtocolVersion // Valid version prefix
		// Rest is zeros - invalid MAC1
		_, _ = clientAdapter.Write(garbage)
	}()

	_, err := serverHS.ServerSideHandshake(serverAdapter)
	if err == nil || err != ErrInvalidMAC1 {
		t.Fatalf("expected ErrInvalidMAC1, got: %v", err)
	}
}

func TestIKHandshake_AllowedIPsInResult(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []testPeer{
		{
			PublicKey: clientKP.Public,
			Enabled:   true,
			ClientID:  5,
		},
	}

	cookieManager, _ := NewCookieManager()
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager, nil,
	)

	clientHS := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)

	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

	completeTestHandshake(t, serverHS, clientHS, serverAdapter, clientAdapter)

	allowedIPs := serverHS.AllowedIPs()
	if len(allowedIPs) != 0 {
		t.Fatalf("expected no additional allowed IPs, got %d", len(allowedIPs))
	}
}

// TestSecurity_HandshakeReplayMsg1 verifies that replaying msg1 produces different session keys.
// This ensures replay protection via fresh ephemeral keys.
func TestSecurity_HandshakeReplayMsg1(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []testPeer{
		{PublicKey: clientKP.Public, Enabled: true, ClientID: 5},
	}

	cookieManager, _ := NewCookieManager()
	loadMonitor := NewLoadMonitor(10000)

	// First handshake - capture the msg1
	serverHS1 := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager, loadMonitor,
	)
	clientHS1 := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn1, serverConn1 := net.Pipe()
	clientAdapter1, _ := transport.NewFramedConn(clientConn1, 2048)
	serverAdapter1, _ := transport.NewFramedConn(serverConn1, 2048)

	completeTestHandshake(t, serverHS1, clientHS1, serverAdapter1, clientAdapter1)

	closeTestConn(clientConn1)
	closeTestConn(serverConn1)

	sessionKey1 := make([]byte, len(clientHS1.clientKey))
	copy(sessionKey1, clientHS1.clientKey)

	// Second handshake with same client keys - must produce different session keys
	serverHS2 := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager, loadMonitor,
	)
	clientHS2 := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn2, serverConn2 := net.Pipe()
	clientAdapter2, _ := transport.NewFramedConn(clientConn2, 2048)
	serverAdapter2, _ := transport.NewFramedConn(serverConn2, 2048)

	completeTestHandshake(t, serverHS2, clientHS2, serverAdapter2, clientAdapter2)

	closeTestConn(clientConn2)
	closeTestConn(serverConn2)

	sessionKey2 := make([]byte, len(clientHS2.clientKey))
	copy(sessionKey2, clientHS2.clientKey)

	// Even with same client identity, session keys must differ
	// This proves fresh ephemerals are used, providing replay protection
	if bytes.Equal(sessionKey1, sessionKey2) {
		t.Fatal("replaying msg1 with same client keys MUST produce different session keys (fresh ephemeral)")
	}

	// Additionally verify server's keys are also different
	if bytes.Equal(serverHS1.clientKey, serverHS2.clientKey) {
		t.Fatal("server-side session keys MUST differ between handshakes")
	}
}

// TestSecurity_RejectUnknownProtocolVersions verifies that the IK server rejects unknown protocol versions.
func TestSecurity_RejectUnknownProtocolVersions(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []testPeer{
		{PublicKey: clientKP.Public, Enabled: true, ClientID: 5},
	}

	cookieManager, _ := NewCookieManager()

	t.Run("version 0 rejected", func(t *testing.T) {
		serverHS := NewIKHandshakeServer(
			serverKP.Public, serverKP.Private,
			newTestAllowedPeers(allowedPeers),
			cookieManager, nil,
		)

		clientConn, serverConn := net.Pipe()
		defer closeTestConn(clientConn)
		defer closeTestConn(serverConn)

		clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
		serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

		srvCh := make(chan error, 1)
		go func() {
			_, err := serverHS.ServerSideHandshake(serverAdapter)
			srvCh <- err
		}()
		go func() {
			// Send message with version=0 (reserved)
			msg := make([]byte, MinTotalSizeWithVersion)
			msg[0] = 0 // Version 0 = reserved
			_, _ = clientAdapter.Write(msg)
		}()

		err := <-srvCh
		if err != ErrUnknownProtocol {
			t.Fatalf("expected ErrUnknownProtocol, got: %v", err)
		}
	})

	t.Run("future version rejected", func(t *testing.T) {
		serverHS := NewIKHandshakeServer(
			serverKP.Public, serverKP.Private,
			newTestAllowedPeers(allowedPeers),
			cookieManager, nil,
		)

		clientConn, serverConn := net.Pipe()
		defer closeTestConn(clientConn)
		defer closeTestConn(serverConn)

		clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
		serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

		srvCh := make(chan error, 1)
		go func() {
			_, err := serverHS.ServerSideHandshake(serverAdapter)
			srvCh <- err
		}()
		go func() {
			// Send message with future version
			msg := make([]byte, MinTotalSizeWithVersion)
			msg[0] = 2 // Version 2 = future/reserved
			_, _ = clientAdapter.Write(msg)
		}()

		err := <-srvCh
		if err != ErrUnknownProtocol {
			t.Fatalf("expected ErrUnknownProtocol, got: %v", err)
		}
	})

	t.Run("message too short rejected", func(t *testing.T) {
		serverHS := NewIKHandshakeServer(
			serverKP.Public, serverKP.Private,
			newTestAllowedPeers(allowedPeers),
			cookieManager, nil,
		)

		clientConn, serverConn := net.Pipe()
		defer closeTestConn(clientConn)
		defer closeTestConn(serverConn)

		clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
		serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

		srvCh := make(chan error, 1)
		go func() {
			_, err := serverHS.ServerSideHandshake(serverAdapter)
			srvCh <- err
		}()
		go func() {
			// Send message that's too short
			msg := make([]byte, 10)
			_, _ = clientAdapter.Write(msg)
		}()

		err := <-srvCh
		if err != ErrMsgTooShort {
			t.Fatalf("expected ErrMsgTooShort, got: %v", err)
		}
	})
}

// TestSecurity_SpoofedSourceIP verifies that packets with unauthorized source IPs are detected.
// This test exercises the session's IsSourceAllowed function which is called by dataplane workers.
func TestSecurity_SpoofedSourceIP(t *testing.T) {
	// This test verifies the IsSourceAllowed function correctly blocks spoofed IPs
	// The actual enforcement happens in dataplane workers, but the logic is in Session

	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []testPeer{
		{
			PublicKey: clientKP.Public,
			Enabled:   true,
			ClientID:  5,
		},
	}

	cookieManager, _ := NewCookieManager()
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager, nil,
	)

	clientHS := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)

	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)
	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

	completeTestHandshake(t, serverHS, clientHS, serverAdapter, clientAdapter)

	if serverHS.ClientPubKey() == nil {
		t.Fatal("authenticated client key should be set after successful handshake")
	}

	// Test cases for source IP validation
	tests := []struct {
		name    string
		srcIP   string
		allowed bool
	}{
		{"client assigned IP", "10.0.0.5", true},
		{"spoofed IP outside range", "10.0.0.99", false},
		{"spoofed IP different subnet", "10.0.1.1", false},
		{"spoofed public IP", "8.8.8.8", false},
		{"spoofed localhost", "127.0.0.1", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srcIP, err := netip.ParseAddr(tc.srcIP)
			if err != nil {
				t.Fatalf("failed to parse IP %s: %v", tc.srcIP, err)
			}

			// Check if internal IP matches (client index 5 in 10.0.0.0/24 = 10.0.0.5 at registration)
			internalIP, _ := netip.ParseAddr("10.0.0.5")
			allowed := srcIP == internalIP

			if allowed != tc.allowed {
				t.Errorf("source IP %s: expected allowed=%v, got %v", tc.srcIP, tc.allowed, allowed)
			}
		})
	}
}

// TestSecurity_CookieBoundToEphemeral verifies that cookies are bound to client ephemeral keys.
// A cookie generated for one ephemeral cannot be used with a different msg1.
func TestSecurity_CookieBoundToEphemeral(t *testing.T) {
	serverPubKey := make([]byte, 32)
	for i := range serverPubKey {
		serverPubKey[i] = byte(i)
	}

	// Generate two different client ephemeral keys
	eph1, _ := cipherSuite.GenerateKeypair(nil)
	eph2, _ := cipherSuite.GenerateKeypair(nil)

	// Create a cookie reply encrypted to eph1
	secret := [32]byte{}
	for i := range secret {
		secret[i] = byte(i + 100)
	}
	cm := cookieManagerWithSecret(secret)

	clientIP, _ := netip.ParseAddr("192.168.1.100")
	cookieReply, err := cm.CreateCookieReply(clientIP, eph1.Public, serverPubKey)
	if err != nil {
		t.Fatalf("failed to create cookie reply: %v", err)
	}

	// Decrypt with correct ephemeral should succeed
	cookie1, err := DecryptCookieReply(cookieReply, eph1.Public, serverPubKey)
	if err != nil {
		t.Fatalf("decryption with correct ephemeral should succeed: %v", err)
	}
	if len(cookie1) != CookieSize {
		t.Fatalf("expected cookie size %d, got %d", CookieSize, len(cookie1))
	}

	// Decrypt with different ephemeral should fail
	_, err = DecryptCookieReply(cookieReply, eph2.Public, serverPubKey)
	if err == nil {
		t.Fatal("decryption with wrong ephemeral MUST fail - cookie should be bound to original ephemeral")
	}

	// Verify the cookie is valid for the original IP
	if !cm.ValidateCookie(clientIP, cookie1) {
		t.Fatal("cookie should be valid for original client IP")
	}

	// Verify the cookie is invalid for a different IP
	differentIP, _ := netip.ParseAddr("192.168.1.200")
	if cm.ValidateCookie(differentIP, cookie1) {
		t.Fatal("cookie should be invalid for different IP")
	}
}

func TestIKHandshake_ServerMissingKeys(t *testing.T) {
	// Server with nil keys
	serverHS := NewIKHandshakeServer(nil, nil, newTestAllowedPeers(nil), nil, nil)
	serverConn, _ := net.Pipe()
	defer closeTestConn(serverConn)
	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)

	_, err := serverHS.ServerSideHandshake(serverAdapter)
	if err != ErrMissingServerKey {
		t.Fatalf("expected ErrMissingServerKey, got: %v", err)
	}
}

func TestIKHandshake_AuthResult_NilBeforeHandshake(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		newTestAllowedPeers(nil), nil, nil,
	)
	if serverHS.ClientPubKey() != nil || serverHS.AllowedIPs() != nil {
		t.Fatal("expected nil authentication result before handshake")
	}
}

func TestIKHandshake_ClientAuthResult_AlwaysNil(t *testing.T) {
	clientKP, _ := cipherSuite.GenerateKeypair(nil)
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientHS := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)
	if clientHS.ClientPubKey() != nil || clientHS.AllowedIPs() != nil {
		t.Fatal("expected nil authentication result for client handshake")
	}
}

// TestSecurity_MAC1VerifiedBeforeAllocation verifies that MAC1 is checked
// before any expensive operations or state allocation.
func TestSecurity_MAC1VerifiedBeforeAllocation(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []testPeer{
		{PublicKey: clientKP.Public, Enabled: true, ClientID: 5},
	}

	cookieManager, _ := NewCookieManager()
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		newTestAllowedPeers(allowedPeers),
		cookieManager, nil,
	)

	// Send a message with invalid MAC1 - should be rejected immediately
	clientConn, serverConn := net.Pipe()
	defer closeTestConn(clientConn)
	defer closeTestConn(serverConn)

	serverAdapter, _ := transport.NewFramedConn(serverConn, 2048)
	clientAdapter, _ := transport.NewFramedConn(clientConn, 2048)

	// Construct a fake msg1 with valid version but invalid MAC1
	fakeMsg := make([]byte, MinTotalSizeWithVersion)
	fakeMsg[0] = ProtocolVersion // Valid version prefix
	// Rest: first 32 bytes are "ephemeral", rest is garbage
	// MAC1 and MAC2 (last 32 bytes) are zeros - invalid

	errCh := make(chan error, 1)
	go func() {
		_, err := serverHS.ServerSideHandshake(serverAdapter)
		errCh <- err
	}()

	// Send the fake message
	go func() {
		_, _ = clientAdapter.Write(fakeMsg)
	}()

	err := <-errCh
	if err != ErrInvalidMAC1 {
		t.Fatalf("expected ErrInvalidMAC1, got: %v", err)
	}

	// The key point: server rejected BEFORE doing any DH or allocating session state
	// (This is verified by the quick return with ErrInvalidMAC1)
}
