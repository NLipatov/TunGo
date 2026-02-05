package noise

import (
	"bytes"
	"fmt"
	"net/netip"
	"sync/atomic"
	"tungo/application/network/connection"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/mem"
	"tungo/infrastructure/settings"

	noiselib "github.com/flynn/noise"
)

var cipherSuite = noiselib.NewCipherSuite(noiselib.DH25519, noiselib.CipherChaChaPoly, noiselib.HashSHA256)

// IKHandshakeResult contains the result of a successful server-side IK handshake.
// Implements connection.HandshakeResult interface.
type IKHandshakeResult struct {
	// clientIP is the server-assigned internal IP for this client.
	clientIP netip.Addr

	// clientPubKey is the client's X25519 static public key.
	clientPubKey []byte

	// allowedIPs are the additional prefixes this client may use as source IP.
	allowedIPs []netip.Prefix
}

// ClientIP returns the server-assigned internal IP for this client.
func (r *IKHandshakeResult) ClientIP() netip.Addr {
	return r.clientIP
}

// ClientPubKey returns the client's X25519 static public key.
func (r *IKHandshakeResult) ClientPubKey() []byte {
	return r.clientPubKey
}

// AllowedIPs returns the additional prefixes this client may use as source IP.
func (r *IKHandshakeResult) AllowedIPs() []netip.Prefix {
	return r.allowedIPs
}

// AllowedPeersLookup provides lookup functionality for AllowedPeers.
type AllowedPeersLookup interface {
	// Lookup returns the peer configuration for the given public key.
	// Returns nil if the peer is not found.
	Lookup(pubKey []byte) *server.AllowedPeer

	// Update atomically replaces the peer map with a new configuration.
	// This allows runtime updates without server restart.
	Update(peers []server.AllowedPeer)
}

// allowedPeersMap implements AllowedPeersLookup using atomic pointer for lock-free reads.
type allowedPeersMap struct {
	peers atomic.Pointer[map[string]*server.AllowedPeer]
}

// NewAllowedPeersLookup creates an AllowedPeersLookup from a slice of AllowedPeer.
func NewAllowedPeersLookup(peers []server.AllowedPeer) AllowedPeersLookup {
	a := &allowedPeersMap{}
	a.Update(peers)
	return a
}

func (a *allowedPeersMap) Lookup(pubKey []byte) *server.AllowedPeer {
	m := a.peers.Load()
	if m == nil {
		return nil
	}
	return (*m)[string(pubKey)]
}

func (a *allowedPeersMap) Update(peers []server.AllowedPeer) {
	m := make(map[string]*server.AllowedPeer, len(peers))
	for i := range peers {
		m[string(peers[i].PublicKey)] = &peers[i]
	}
	a.peers.Store(&m)
}

// IKHandshake implements Noise IK handshake with DoS protection.
type IKHandshake struct {
	id        [32]byte
	clientKey []byte
	serverKey []byte

	// Server-side fields
	serverPubKey  []byte
	serverPrivKey []byte
	allowedPeers  AllowedPeersLookup
	cookieManager *CookieManager
	loadMonitor   *LoadMonitor

	// Client-side fields
	clientPubKey  []byte
	clientPrivKey []byte
	peerPubKey    []byte // Server's public key (client perspective)

	// Handshake result (server-side)
	result *IKHandshakeResult

	// Cookie for retry (client-side)
	cookie []byte
}

// NewIKHandshakeServer creates a new IK handshake for server-side use.
func NewIKHandshakeServer(
	serverPubKey, serverPrivKey []byte,
	allowedPeers AllowedPeersLookup,
	cookieManager *CookieManager,
	loadMonitor *LoadMonitor,
) *IKHandshake {
	return &IKHandshake{
		serverPubKey:  serverPubKey,
		serverPrivKey: serverPrivKey,
		allowedPeers:  allowedPeers,
		cookieManager: cookieManager,
		loadMonitor:   loadMonitor,
	}
}

// NewIKHandshakeClient creates a new IK handshake for client-side use.
func NewIKHandshakeClient(
	clientPubKey, clientPrivKey []byte,
	serverPubKey []byte,
) *IKHandshake {
	return &IKHandshake{
		clientPubKey:  clientPubKey,
		clientPrivKey: clientPrivKey,
		peerPubKey:    serverPubKey,
	}
}

func (h *IKHandshake) Id() [32]byte              { return h.id }
func (h *IKHandshake) KeyClientToServer() []byte { return h.clientKey }
func (h *IKHandshake) KeyServerToClient() []byte { return h.serverKey }

// Result returns the handshake result (server-side only).
// Implements connection.HandshakeWithResult interface.
func (h *IKHandshake) Result() connection.HandshakeResult {
	if h.result == nil {
		return nil
	}
	return h.result
}

// ServerSideHandshake performs Noise IK as responder with DoS protection.
// Returns the client's server-assigned internal IP.
func (h *IKHandshake) ServerSideHandshake(transport connection.Transport) (netip.Addr, error) {
	if h.serverPrivKey == nil || h.serverPubKey == nil {
		return netip.Addr{}, ErrMissingServerKey
	}
	if h.allowedPeers == nil {
		return netip.Addr{}, ErrMissingAllowedPeers
	}

	// Read msg1 with version prefix and MACs
	msg1Buf := make([]byte, 2048)
	n, err := transport.Read(msg1Buf)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("noise: read msg1: %w", err)
	}
	msgWithVersion := msg1Buf[:n]

	// PHASE 0: Check protocol version BEFORE any crypto
	// This rejects deprecated (v1/XX) and unknown versions immediately.
	msg1WithMAC, err := CheckVersion(msgWithVersion)
	if err != nil {
		return netip.Addr{}, err
	}

	// PHASE 1: Verify MAC1 (stateless, cheap) BEFORE any allocation
	if !VerifyMAC1(msg1WithMAC, h.serverPubKey) {
		return netip.Addr{}, ErrInvalidMAC1
	}

	// Record handshake for load monitoring
	if h.loadMonitor != nil {
		h.loadMonitor.RecordHandshake()
	}

	// PHASE 2: Check load and MAC2
	if h.loadMonitor != nil && h.loadMonitor.UnderLoad() && h.cookieManager != nil {
		// Extract client ephemeral AFTER MAC1 verification
		clientEphemeral := ExtractClientEphemeral(msg1WithMAC)
		if clientEphemeral == nil {
			return netip.Addr{}, ErrMsgTooShort
		}

		// Extract client IP from transport for cookie binding
		var clientIP netip.Addr
		if tr, ok := transport.(connection.TransportWithRemoteAddr); ok {
			clientIP = tr.RemoteAddrPort().Addr()
		} else {
			// Fallback: cannot bind cookie to IP, reject under load
			return netip.Addr{}, ErrCookieRequired
		}

		cookie := h.cookieManager.ComputeCookieValue(clientIP)
		if !VerifyMAC2(msg1WithMAC, cookie) {
			// Send cookie reply
			reply, err := h.cookieManager.CreateCookieReply(clientIP, clientEphemeral, h.serverPubKey)
			if err != nil {
				return netip.Addr{}, fmt.Errorf("noise: create cookie reply: %w", err)
			}
			if _, err := transport.Write(reply); err != nil {
				return netip.Addr{}, fmt.Errorf("noise: send cookie reply: %w", err)
			}
			return netip.Addr{}, ErrCookieRequired
		}
	}

	// PHASE 3: Process Noise handshake
	noiseMsg := ExtractNoiseMsg(msg1WithMAC)

	staticKey := noiselib.DHKey{
		Private: h.serverPrivKey,
		Public:  h.serverPubKey,
	}

	hs, err := noiselib.NewHandshakeState(noiselib.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noiselib.HandshakeIK,
		Initiator:     false,
		StaticKeypair: staticKey,
	})
	if err != nil {
		return netip.Addr{}, fmt.Errorf("noise: server handshake state: %w", err)
	}

	// Zero ephemeral on any exit path (set early before WriteMessage)
	defer func() {
		if localEph := hs.LocalEphemeral(); localEph.Private != nil {
			mem.ZeroBytes(localEph.Private)
		}
	}()

	// Read msg1 (e, es, s, ss)
	_, _, _, err = hs.ReadMessage(nil, noiseMsg)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("noise: read msg1: %w", err)
	}

	// Extract and validate client identity
	clientPubKey := hs.PeerStatic()
	peer := h.allowedPeers.Lookup(clientPubKey)
	if peer == nil {
		return netip.Addr{}, ErrUnknownPeer
	}
	if !peer.Enabled {
		return netip.Addr{}, ErrPeerDisabled
	}

	// Write msg2 (e, ee, se)
	msg2, cs1, cs2, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("noise: write msg2: %w", err)
	}

	if _, err := transport.Write(msg2); err != nil {
		return netip.Addr{}, fmt.Errorf("noise: send msg2: %w", err)
	}

	if cs1 == nil || cs2 == nil {
		return netip.Addr{}, fmt.Errorf("noise: handshake not complete after msg2")
	}

	// Extract session keys
	c2sKey := cs1.UnsafeKey()
	s2cKey := cs2.UnsafeKey()

	h.clientKey = make([]byte, 32)
	copy(h.clientKey, c2sKey[:])
	h.serverKey = make([]byte, 32)
	copy(h.serverKey, s2cKey[:])
	mem.ZeroBytes(c2sKey[:])
	mem.ZeroBytes(s2cKey[:])

	// Session ID from channel binding
	cb := hs.ChannelBinding()
	copy(h.id[:], cb[:32])

	// Parse client IP and prepare result
	clientIP, err := netip.ParseAddr(peer.ClientIP)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("noise: invalid client IP in config: %s", peer.ClientIP)
	}

	pubKeyCopy := make([]byte, len(clientPubKey))
	copy(pubKeyCopy, clientPubKey)

	h.result = &IKHandshakeResult{
		clientIP:     clientIP,
		clientPubKey: pubKeyCopy,
		allowedIPs:   peer.AllowedIPPrefixes(),
	}

	return clientIP, nil
}

// ClientSideHandshake performs Noise IK as initiator.
func (h *IKHandshake) ClientSideHandshake(transport connection.Transport, s settings.Settings) error {
	if h.clientPrivKey == nil || h.clientPubKey == nil {
		return ErrMissingClientKey
	}
	if h.peerPubKey == nil {
		return ErrMissingServerKey
	}

	clientStatic := noiselib.DHKey{
		Private: h.clientPrivKey,
		Public:  h.clientPubKey,
	}

	hs, err := noiselib.NewHandshakeState(noiselib.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noiselib.HandshakeIK,
		Initiator:     true,
		StaticKeypair: clientStatic,
		PeerStatic:    h.peerPubKey,
	})
	if err != nil {
		return fmt.Errorf("noise: client handshake state: %w", err)
	}

	// Zero ephemeral on any exit path (set early before WriteMessage)
	defer func() {
		if localEph := hs.LocalEphemeral(); localEph.Private != nil {
			mem.ZeroBytes(localEph.Private)
		}
	}()

	// Generate msg1 (e, es, s, ss)
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return fmt.Errorf("noise: write msg1: %w", err)
	}

	// Add MACs (MAC2 is zeros if no cookie, or valid if we have one)
	msg1WithMAC := AppendMACs(msg1, h.peerPubKey, h.cookie)

	// Prepend protocol version byte
	msgWithVersion := PrependVersion(msg1WithMAC)

	if _, err := transport.Write(msgWithVersion); err != nil {
		return fmt.Errorf("noise: send msg1: %w", err)
	}

	// Read response (could be msg2 or cookie reply)
	responseBuf := make([]byte, 2048)
	n, err := transport.Read(responseBuf)
	if err != nil {
		return fmt.Errorf("noise: read response: %w", err)
	}
	response := responseBuf[:n]

	// Check if it's a cookie reply
	if IsCookieReply(response) {
		cookie, err := DecryptCookieReply(response, hs.LocalEphemeral().Public, h.peerPubKey)
		if err != nil {
			return fmt.Errorf("noise: decrypt cookie: %w", err)
		}
		h.cookie = cookie

		// Retry with cookie - need to create new handshake state
		return h.retryWithCookie(transport, s)
	}

	// Process msg2 (e, ee, se)
	_, cs1, cs2, err := hs.ReadMessage(nil, response)
	if err != nil {
		return fmt.Errorf("noise: read msg2: %w", err)
	}

	if cs1 == nil || cs2 == nil {
		return fmt.Errorf("noise: handshake not complete after msg2")
	}

	// Verify server's static key matches expected
	peerStatic := hs.PeerStatic()
	if !bytes.Equal(peerStatic, h.peerPubKey) {
		return fmt.Errorf("noise: server static key mismatch")
	}

	// Extract session keys
	c2sKey := cs1.UnsafeKey()
	s2cKey := cs2.UnsafeKey()

	h.clientKey = make([]byte, 32)
	copy(h.clientKey, c2sKey[:])
	h.serverKey = make([]byte, 32)
	copy(h.serverKey, s2cKey[:])
	mem.ZeroBytes(c2sKey[:])
	mem.ZeroBytes(s2cKey[:])

	cb := hs.ChannelBinding()
	copy(h.id[:], cb[:32])

	return nil
}

// retryWithCookie retries the handshake with the stored cookie.
func (h *IKHandshake) retryWithCookie(transport connection.Transport, s settings.Settings) error {
	clientStatic := noiselib.DHKey{
		Private: h.clientPrivKey,
		Public:  h.clientPubKey,
	}

	hs, err := noiselib.NewHandshakeState(noiselib.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noiselib.HandshakeIK,
		Initiator:     true,
		StaticKeypair: clientStatic,
		PeerStatic:    h.peerPubKey,
	})
	if err != nil {
		return fmt.Errorf("noise: client handshake state: %w", err)
	}

	// Zero ephemeral on any exit path (set early before WriteMessage)
	defer func() {
		if localEph := hs.LocalEphemeral(); localEph.Private != nil {
			mem.ZeroBytes(localEph.Private)
		}
	}()

	// Generate new msg1 with fresh ephemeral
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return fmt.Errorf("noise: write msg1: %w", err)
	}

	// Add MACs with cookie
	msg1WithMAC := AppendMACs(msg1, h.peerPubKey, h.cookie)

	// Prepend protocol version byte
	msgWithVersion := PrependVersion(msg1WithMAC)

	if _, err := transport.Write(msgWithVersion); err != nil {
		return fmt.Errorf("noise: send msg1 retry: %w", err)
	}

	// Read msg2
	responseBuf := make([]byte, 2048)
	n, err := transport.Read(responseBuf)
	if err != nil {
		return fmt.Errorf("noise: read msg2: %w", err)
	}
	response := responseBuf[:n]

	// Should not get another cookie reply
	if IsCookieReply(response) {
		return fmt.Errorf("noise: unexpected cookie reply on retry")
	}

	// Process msg2
	_, cs1, cs2, err := hs.ReadMessage(nil, response)
	if err != nil {
		return fmt.Errorf("noise: read msg2: %w", err)
	}

	if cs1 == nil || cs2 == nil {
		return fmt.Errorf("noise: handshake not complete after msg2")
	}

	// Extract session keys
	c2sKey := cs1.UnsafeKey()
	s2cKey := cs2.UnsafeKey()

	h.clientKey = make([]byte, 32)
	copy(h.clientKey, c2sKey[:])
	h.serverKey = make([]byte, 32)
	copy(h.serverKey, s2cKey[:])
	mem.ZeroBytes(c2sKey[:])
	mem.ZeroBytes(s2cKey[:])

	cb := hs.ChannelBinding()
	copy(h.id[:], cb[:32])

	return nil
}
