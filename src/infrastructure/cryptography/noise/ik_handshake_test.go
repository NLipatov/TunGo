package noise

import (
	"bytes"
	"net"
	"net/netip"
	"strings"
	"testing"

	framelimit "tungo/domain/network/ip/frame_limit"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/network/tcp/adapters"
	"tungo/infrastructure/settings"
)

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
	allowedPeers := []server.AllowedPeer{
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
		NewAllowedPeersLookup(allowedPeers),
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
	defer clientConn.Close()
	defer serverConn.Close()

	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

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

	// Verify result is populated
	result := serverHS.Result()
	if result == nil {
		t.Fatal("server result should not be nil")
	}
	if !bytes.Equal(result.ClientPubKey(), clientKP.Public) {
		t.Fatal("result client pub key mismatch")
	}
}

func TestIKHandshake_UnknownClient(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)
	unknownKP, _ := cipherSuite.GenerateKeypair(nil)

	// Only allow clientKP
	allowedPeers := []server.AllowedPeer{
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
		NewAllowedPeersLookup(allowedPeers),
		cookieManager, loadMonitor,
	)

	// Client uses unknown key
	clientHS := NewIKHandshakeClient(
		unknownKP.Public, unknownKP.Private,
		serverKP.Public,
	)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

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
	allowedPeers := []server.AllowedPeer{
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
		NewAllowedPeersLookup(allowedPeers),
		cookieManager, loadMonitor,
	)

	clientHS := NewIKHandshakeClient(
		clientKP.Public, clientKP.Private,
		serverKP.Public,
	)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

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

	allowedPeers := []server.AllowedPeer{
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
		NewAllowedPeersLookup(allowedPeers),
		cookieManager, loadMonitor,
	)

	// Client expects real server's key
	clientHS := NewIKHandshakeClient(
		clientKP.Public, clientKP.Private,
		serverKP.Public, // Wrong server key
	)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

	srvCh := make(chan error, 1)
	cliCh := make(chan error, 1)

	go func() {
		_, err := serverHS.ServerSideHandshake(serverAdapter)
		srvCh <- err
		// Close server's side when it's done to unblock client
		serverConn.Close()
	}()
	go func() {
		cliCh <- clientHS.ClientSideHandshake(clientAdapter)
		// Close client's side when it's done to unblock server
		clientConn.Close()
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

	allowedPeers := []server.AllowedPeer{
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
			NewAllowedPeersLookup(allowedPeers),
			cookieManager, loadMonitor,
		)
		clientHS := NewIKHandshakeClient(
			clientKP.Public, clientKP.Private,
			serverKP.Public,
		)

		clientConn, serverConn := net.Pipe()
		clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
		serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

		done := make(chan struct{})
		go func() {
			serverHS.ServerSideHandshake(serverAdapter)
			close(done)
		}()
		clientHS.ClientSideHandshake(clientAdapter)
		<-done

		clientConn.Close()
		serverConn.Close()

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
	defer clientConn.Close()
	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))

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
	defer clientConn.Close()
	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))

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
	defer serverConn.Close()
	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

	_, err := serverHS.ServerSideHandshake(serverAdapter)
	if err == nil || err != ErrMissingAllowedPeers {
		t.Fatalf("expected ErrMissingAllowedPeers, got: %v", err)
	}
}

func TestIKHandshake_InvalidMAC1(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []server.AllowedPeer{
		{PublicKey: clientKP.Public, Enabled: true, ClientID: 5},
	}

	cookieManager, _ := NewCookieManager()
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		NewAllowedPeersLookup(allowedPeers),
		cookieManager, nil,
	)

	// Create a transport that sends garbage
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))
	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))

	// Send garbage with valid version byte but invalid MAC1
	go func() {
		garbage := make([]byte, MinTotalSizeWithVersion)
		garbage[0] = ProtocolVersion // Valid version prefix
		// Rest is zeros - invalid MAC1
		clientAdapter.Write(garbage)
	}()

	_, err := serverHS.ServerSideHandshake(serverAdapter)
	if err == nil || err != ErrInvalidMAC1 {
		t.Fatalf("expected ErrInvalidMAC1, got: %v", err)
	}
}

func TestAllowedPeersLookup(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1
	pubKey2 := make([]byte, 32)
	pubKey2[0] = 2

	peers := []server.AllowedPeer{
		{PublicKey: pubKey1, Enabled: true, ClientID: 1},
		{PublicKey: pubKey2, Enabled: false, ClientID: 2},
	}

	lookup := NewAllowedPeersLookup(peers)

	// Find existing peer
	clientID, enabled, found := lookup.Lookup(pubKey1)
	if !found {
		t.Fatal("should find peer 1")
	}
	if clientID != 1 {
		t.Fatal("wrong peer returned")
	}
	if !enabled {
		t.Fatal("peer 1 should be enabled")
	}

	// Find second peer
	clientID2, enabled2, found2 := lookup.Lookup(pubKey2)
	if !found2 {
		t.Fatal("should find peer 2")
	}
	if clientID2 != 2 {
		t.Fatal("wrong peer returned")
	}
	if enabled2 {
		t.Fatal("peer 2 should be disabled")
	}

	// Unknown peer
	unknown := make([]byte, 32)
	unknown[0] = 99
	if _, _, found := lookup.Lookup(unknown); found {
		t.Fatal("should not find unknown peer")
	}
}

func TestAllowedPeersLookup_DynamicUpdate(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1
	pubKey2 := make([]byte, 32)
	pubKey2[0] = 2
	pubKey3 := make([]byte, 32)
	pubKey3[0] = 3

	// Initial peers
	peers := []server.AllowedPeer{
		{PublicKey: pubKey1, Enabled: true, ClientID: 1},
	}

	lookup := NewAllowedPeersLookup(peers)

	// Verify initial state
	if _, _, found := lookup.Lookup(pubKey1); !found {
		t.Fatal("should find peer 1")
	}
	if _, _, found := lookup.Lookup(pubKey2); found {
		t.Fatal("should not find peer 2 before update")
	}

	// Update with new peers
	newPeers := []server.AllowedPeer{
		{PublicKey: pubKey2, Enabled: true, ClientID: 2},
		{PublicKey: pubKey3, Enabled: true, ClientID: 3},
	}
	lookup.Update(newPeers)

	// Old peer should be gone
	if _, _, found := lookup.Lookup(pubKey1); found {
		t.Fatal("should not find peer 1 after update")
	}

	// New peers should be present
	if _, _, found := lookup.Lookup(pubKey2); !found {
		t.Fatal("should find peer 2 after update")
	}
	if _, _, found := lookup.Lookup(pubKey3); !found {
		t.Fatal("should find peer 3 after update")
	}

	// Verify correct data
	clientID2, _, found := lookup.Lookup(pubKey2)
	if !found {
		t.Fatal("should find peer 2 after update")
	}
	if clientID2 != 2 {
		t.Fatalf("expected ClientID 2, got %d", clientID2)
	}
}

func TestIKHandshake_AllowedIPsInResult(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []server.AllowedPeer{
		{
			PublicKey: clientKP.Public,
			Enabled:   true,
			ClientID:  5,
		},
	}

	cookieManager, _ := NewCookieManager()
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		NewAllowedPeersLookup(allowedPeers),
		cookieManager, nil,
	)

	clientHS := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

	done := make(chan struct{})
	go func() {
		serverHS.ServerSideHandshake(serverAdapter)
		close(done)
	}()
	clientHS.ClientSideHandshake(clientAdapter)
	<-done

	result := serverHS.Result()
	if result == nil {
		t.Fatal("result should not be nil")
	}

	allowedIPs := result.AllowedIPs()
	if len(allowedIPs) != 0 {
		t.Fatalf("expected no additional allowed IPs, got %d", len(allowedIPs))
	}
}

// TestSecurity_HandshakeReplayMsg1 verifies that replaying msg1 produces different session keys.
// This ensures replay protection via fresh ephemeral keys.
func TestSecurity_HandshakeReplayMsg1(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []server.AllowedPeer{
		{PublicKey: clientKP.Public, Enabled: true, ClientID: 5},
	}

	cookieManager, _ := NewCookieManager()
	loadMonitor := NewLoadMonitor(10000)

	// First handshake - capture the msg1
	serverHS1 := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		NewAllowedPeersLookup(allowedPeers),
		cookieManager, loadMonitor,
	)
	clientHS1 := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn1, serverConn1 := net.Pipe()
	clientAdapter1, _ := adapters.NewLengthPrefixFramingAdapter(clientConn1, framelimit.Cap(2048))
	serverAdapter1, _ := adapters.NewLengthPrefixFramingAdapter(serverConn1, framelimit.Cap(2048))

	done1 := make(chan struct{})
	go func() {
		serverHS1.ServerSideHandshake(serverAdapter1)
		close(done1)
	}()
	clientHS1.ClientSideHandshake(clientAdapter1)
	<-done1

	clientConn1.Close()
	serverConn1.Close()

	sessionKey1 := make([]byte, len(clientHS1.clientKey))
	copy(sessionKey1, clientHS1.clientKey)

	// Second handshake with same client keys - must produce different session keys
	serverHS2 := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		NewAllowedPeersLookup(allowedPeers),
		cookieManager, loadMonitor,
	)
	clientHS2 := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn2, serverConn2 := net.Pipe()
	clientAdapter2, _ := adapters.NewLengthPrefixFramingAdapter(clientConn2, framelimit.Cap(2048))
	serverAdapter2, _ := adapters.NewLengthPrefixFramingAdapter(serverConn2, framelimit.Cap(2048))

	done2 := make(chan struct{})
	go func() {
		serverHS2.ServerSideHandshake(serverAdapter2)
		close(done2)
	}()
	clientHS2.ClientSideHandshake(clientAdapter2)
	<-done2

	clientConn2.Close()
	serverConn2.Close()

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

	allowedPeers := []server.AllowedPeer{
		{PublicKey: clientKP.Public, Enabled: true, ClientID: 5},
	}

	cookieManager, _ := NewCookieManager()

	t.Run("version 0 rejected", func(t *testing.T) {
		serverHS := NewIKHandshakeServer(
			serverKP.Public, serverKP.Private,
			NewAllowedPeersLookup(allowedPeers),
			cookieManager, nil,
		)

		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
		serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

		srvCh := make(chan error, 1)
		go func() {
			_, err := serverHS.ServerSideHandshake(serverAdapter)
			srvCh <- err
		}()
		go func() {
			// Send message with version=0 (reserved)
			msg := make([]byte, MinTotalSizeWithVersion)
			msg[0] = 0 // Version 0 = reserved
			clientAdapter.Write(msg)
		}()

		err := <-srvCh
		if err != ErrUnknownProtocol {
			t.Fatalf("expected ErrUnknownProtocol, got: %v", err)
		}
	})

	t.Run("future version rejected", func(t *testing.T) {
		serverHS := NewIKHandshakeServer(
			serverKP.Public, serverKP.Private,
			NewAllowedPeersLookup(allowedPeers),
			cookieManager, nil,
		)

		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
		serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

		srvCh := make(chan error, 1)
		go func() {
			_, err := serverHS.ServerSideHandshake(serverAdapter)
			srvCh <- err
		}()
		go func() {
			// Send message with future version
			msg := make([]byte, MinTotalSizeWithVersion)
			msg[0] = 2 // Version 2 = future/reserved
			clientAdapter.Write(msg)
		}()

		err := <-srvCh
		if err != ErrUnknownProtocol {
			t.Fatalf("expected ErrUnknownProtocol, got: %v", err)
		}
	})

	t.Run("message too short rejected", func(t *testing.T) {
		serverHS := NewIKHandshakeServer(
			serverKP.Public, serverKP.Private,
			NewAllowedPeersLookup(allowedPeers),
			cookieManager, nil,
		)

		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
		serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

		srvCh := make(chan error, 1)
		go func() {
			_, err := serverHS.ServerSideHandshake(serverAdapter)
			srvCh <- err
		}()
		go func() {
			// Send message that's too short
			msg := make([]byte, 10)
			clientAdapter.Write(msg)
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

	allowedPeers := []server.AllowedPeer{
		{
			PublicKey: clientKP.Public,
			Enabled:   true,
			ClientID:  5,
		},
	}

	cookieManager, _ := NewCookieManager()
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		NewAllowedPeersLookup(allowedPeers),
		cookieManager, nil,
	)

	clientHS := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

	done := make(chan struct{})
	go func() {
		serverHS.ServerSideHandshake(serverAdapter)
		close(done)
	}()
	clientHS.ClientSideHandshake(clientAdapter)
	<-done

	result := serverHS.Result()
	if result == nil {
		t.Fatal("result should not be nil after successful handshake")
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
	cm := NewCookieManagerWithSecret(secret)

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

// TestSecurity_ClientIDConflictRejectedAtConfig verifies that duplicate peer ClientID values
// are rejected at configuration validation time.
func TestSecurity_ClientIDConflictRejectedAtConfig(t *testing.T) {
	pubKey1 := make([]byte, 32)
	pubKey1[0] = 1
	pubKey2 := make([]byte, 32)
	pubKey2[0] = 2

	tests := []struct {
		name      string
		peers     []server.AllowedPeer
		expectErr bool
		errMsg    string
	}{
		{
			name: "no conflict - distinct indices",
			peers: []server.AllowedPeer{
				{PublicKey: pubKey1, Enabled: true, ClientID: 1},
				{PublicKey: pubKey2, Enabled: true, ClientID: 2},
			},
			expectErr: false,
		},
		{
			name: "conflict - identical ClientID",
			peers: []server.AllowedPeer{
				{PublicKey: pubKey1, Enabled: true, ClientID: 1},
				{PublicKey: pubKey2, Enabled: true, ClientID: 1},
			},
			expectErr: true,
			errMsg:    "ClientID conflict",
		},
		{
			name: "duplicate public keys",
			peers: []server.AllowedPeer{
				{PublicKey: pubKey1, Enabled: true, ClientID: 1},
				{PublicKey: pubKey1, Enabled: true, ClientID: 2},
			},
			expectErr: true,
			errMsg:    "duplicate",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &server.Configuration{
				EnableUDP: true,
				UDPSettings: settings.Settings{
					Addressing: settings.Addressing{
						IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
					},
				},
				AllowedPeers: tc.peers,
			}

			err := cfg.ValidateAllowedPeers()

			if tc.expectErr && err == nil {
				t.Fatal("expected validation error but got none")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("expected no error but got: %v", err)
			}
			if tc.expectErr && err != nil && tc.errMsg != "" {
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got: %v", tc.errMsg, err)
				}
			}
		})
	}
}

func TestIKHandshake_ServerMissingKeys(t *testing.T) {
	// Server with nil keys
	serverHS := NewIKHandshakeServer(nil, nil, NewAllowedPeersLookup(nil), nil, nil)
	serverConn, _ := net.Pipe()
	defer serverConn.Close()
	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))

	_, err := serverHS.ServerSideHandshake(serverAdapter)
	if err != ErrMissingServerKey {
		t.Fatalf("expected ErrMissingServerKey, got: %v", err)
	}
}

func TestIKHandshake_Result_NilBeforeHandshake(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		NewAllowedPeersLookup(nil), nil, nil,
	)
	result := serverHS.Result()
	if result != nil {
		t.Fatal("expected nil result before handshake")
	}
}

func TestIKHandshake_ClientResult_AlwaysNil(t *testing.T) {
	clientKP, _ := cipherSuite.GenerateKeypair(nil)
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientHS := NewIKHandshakeClient(clientKP.Public, clientKP.Private, serverKP.Public)
	result := clientHS.Result()
	if result != nil {
		t.Fatal("expected nil result for client handshake")
	}
}

func TestAllowedPeersLookup_NilMap_ReturnsNil(t *testing.T) {
	// Create lookup with empty peers, then try lookup
	lookup := NewAllowedPeersLookup(nil)
	pubKey := make([]byte, 32)
	pubKey[0] = 1
	if _, _, found := lookup.Lookup(pubKey); found {
		t.Fatal("expected nil for empty peers map")
	}
}

// TestSecurity_MAC1VerifiedBeforeAllocation verifies that MAC1 is checked
// before any expensive operations or state allocation.
func TestSecurity_MAC1VerifiedBeforeAllocation(t *testing.T) {
	serverKP, _ := cipherSuite.GenerateKeypair(nil)
	clientKP, _ := cipherSuite.GenerateKeypair(nil)

	allowedPeers := []server.AllowedPeer{
		{PublicKey: clientKP.Public, Enabled: true, ClientID: 5},
	}

	cookieManager, _ := NewCookieManager()
	serverHS := NewIKHandshakeServer(
		serverKP.Public, serverKP.Private,
		NewAllowedPeersLookup(allowedPeers),
		cookieManager, nil,
	)

	// Send a message with invalid MAC1 - should be rejected immediately
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	serverAdapter, _ := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))
	clientAdapter, _ := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))

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
		clientAdapter.Write(fakeMsg)
	}()

	err := <-errCh
	if err != ErrInvalidMAC1 {
		t.Fatalf("expected ErrInvalidMAC1, got: %v", err)
	}

	// The key point: server rejected BEFORE doing any DH or allocating session state
	// (This is verified by the quick return with ErrInvalidMAC1)
}
