package noise

import (
	"bytes"
	"fmt"
	"net/netip"
	"sync/atomic"
	"tungo/application/network/connection"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/mem"

	noiselib "github.com/flynn/noise"
)

var cipherSuite = noiselib.NewCipherSuite(noiselib.DH25519, noiselib.CipherChaChaPoly, noiselib.HashSHA256)

// ikHandshakeResult contains the result of a successful server-side IK handshake.
// Implements connection.HandshakeResult interface.
type ikHandshakeResult struct {
	// clientID is the 1-based ordinal for AllocateClientIP.
	clientID int

	// clientPubKey is the client's X25519 static public key.
	clientPubKey []byte

	// allowedIPs are the additional prefixes this client may use as source IP.
	allowedIPs []netip.Prefix
}

// ClientPubKey returns the client's X25519 static public key.
func (r *ikHandshakeResult) ClientPubKey() []byte {
	return r.clientPubKey
}

// AllowedIPs returns the additional prefixes this client may use as source IP.
func (r *ikHandshakeResult) AllowedIPs() []netip.Prefix {
	return r.allowedIPs
}

// AllowedPeersLookup provides lookup functionality for AllowedPeers.
type AllowedPeersLookup interface {
	// Lookup returns peer authz data for the given public key.
	// found=false means unknown peer.
	Lookup(pubKey []byte) (clientID int, enabled bool, found bool)

	// Update atomically replaces the peer map with a new configuration.
	// This allows runtime updates without server restart.
	Update(peers []server.AllowedPeer)
}

// allowedPeersMap implements AllowedPeersLookup using atomic pointer for lock-free reads.
type allowedPeersMap struct {
	peers atomic.Pointer[map[string]allowedPeerEntry]
}

type allowedPeerEntry struct {
	enabled  bool
	clientID int
}

// NewAllowedPeersLookup creates an AllowedPeersLookup from a slice of AllowedPeer.
func NewAllowedPeersLookup(peers []server.AllowedPeer) AllowedPeersLookup {
	a := &allowedPeersMap{}
	a.Update(peers)
	return a
}

func (a *allowedPeersMap) Lookup(pubKey []byte) (int, bool, bool) {
	m := a.peers.Load()
	if m == nil {
		return 0, false, false
	}
	peer, ok := (*m)[string(pubKey)]
	if !ok {
		return 0, false, false
	}
	return peer.clientID, peer.enabled, true
}

func (a *allowedPeersMap) Update(peers []server.AllowedPeer) {
	m := make(map[string]allowedPeerEntry, len(peers))
	for i := range peers {
		peer := peers[i]
		m[string(peer.PublicKey)] = allowedPeerEntry{
			enabled:  peer.Enabled,
			clientID: peer.ClientID,
		}
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
	result *ikHandshakeResult

	// Cookie for retry (client-side)
	cookie []byte
}

type sessionMaterial struct {
	id        [32]byte
	clientKey []byte
	serverKey []byte
}

type serverHandshakeOutcome struct {
	clientID     int
	clientPubKey []byte
	material     sessionMaterial
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
// Returns the client's ClientID for IP allocation at registration time.
func (h *IKHandshake) ServerSideHandshake(transport connection.Transport) (int, error) {
	if err := h.validateServerConfig(); err != nil {
		return 0, err
	}

	msgWithVersion, err := readHandshakeMessage(transport, "noise: read msg1")
	if err != nil {
		return 0, err
	}
	msg1WithMAC, err := CheckVersion(msgWithVersion)
	if err != nil {
		return 0, err
	}
	if !VerifyMAC1(msg1WithMAC, h.serverPubKey) {
		return 0, ErrInvalidMAC1
	}

	if h.loadMonitor != nil {
		h.loadMonitor.RecordHandshake()
	}
	if err := h.enforceCookieIfNeeded(transport, msg1WithMAC); err != nil {
		return 0, err
	}

	outcome, err := h.runResponderNoise(transport, msg1WithMAC)
	if err != nil {
		return 0, err
	}
	h.applySessionMaterial(outcome.material)
	h.result = newIKHandshakeResult(outcome.clientID, outcome.clientPubKey)
	return outcome.clientID, nil
}

// ClientSideHandshake performs Noise IK as initiator.
func (h *IKHandshake) ClientSideHandshake(transport connection.Transport) error {
	if err := h.validateClientConfig(); err != nil {
		return err
	}

	hs, response, err := h.initiatorAttempt(transport, "noise: send msg1", "noise: read response")
	if err != nil {
		return err
	}
	if IsCookieReply(response) {
		cookie, err := DecryptCookieReply(response, hs.LocalEphemeral().Public, h.peerPubKey)
		zeroizeLocalEphemeral(hs)
		if err != nil {
			return fmt.Errorf("noise: decrypt cookie: %w", err)
		}
		h.cookie = cookie

		hs, response, err = h.initiatorAttempt(transport, "noise: send msg1 retry", "noise: read msg2")
		if err != nil {
			return err
		}
		if IsCookieReply(response) {
			zeroizeLocalEphemeral(hs)
			return fmt.Errorf("noise: unexpected cookie reply on retry")
		}
		material, err := h.completeInitiatorFromMsg2(hs, response, false)
		zeroizeLocalEphemeral(hs)
		if err != nil {
			return err
		}
		h.applySessionMaterial(material)
		return nil
	}

	material, err := h.completeInitiatorFromMsg2(hs, response, true)
	zeroizeLocalEphemeral(hs)
	if err != nil {
		return err
	}
	h.applySessionMaterial(material)
	return nil
}

func (h *IKHandshake) validateServerConfig() error {
	if h.serverPrivKey == nil || h.serverPubKey == nil {
		return ErrMissingServerKey
	}
	if h.allowedPeers == nil {
		return ErrMissingAllowedPeers
	}
	return nil
}

func (h *IKHandshake) validateClientConfig() error {
	if h.clientPrivKey == nil || h.clientPubKey == nil {
		return ErrMissingClientKey
	}
	if h.peerPubKey == nil {
		return ErrMissingServerKey
	}
	return nil
}

func (h *IKHandshake) enforceCookieIfNeeded(
	transport connection.Transport,
	msg1WithMAC []byte,
) error {
	if h.loadMonitor == nil || !h.loadMonitor.UnderLoad() || h.cookieManager == nil {
		return nil
	}

	clientEphemeral := ExtractClientEphemeral(msg1WithMAC)
	if clientEphemeral == nil {
		return ErrMsgTooShort
	}

	clientIP, ok := h.transportRemoteIP(transport)
	if !ok {
		return ErrCookieRequired
	}
	if h.cookieManager.VerifyMAC2ForClient(msg1WithMAC, clientIP) {
		return nil
	}

	reply, err := h.cookieManager.CreateCookieReply(clientIP, clientEphemeral, h.serverPubKey)
	if err != nil {
		return fmt.Errorf("noise: create cookie reply: %w", err)
	}
	if _, err := transport.Write(reply); err != nil {
		return fmt.Errorf("noise: send cookie reply: %w", err)
	}
	return ErrCookieRequired
}

func (h *IKHandshake) transportRemoteIP(transport connection.Transport) (netip.Addr, bool) {
	tr, ok := transport.(connection.TransportWithRemoteAddr)
	if !ok {
		return netip.Addr{}, false
	}
	return tr.RemoteAddrPort().Addr(), true
}

func (h *IKHandshake) newResponderState() (*noiselib.HandshakeState, error) {
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
		return nil, fmt.Errorf("noise: server handshake state: %w", err)
	}
	return hs, nil
}

func (h *IKHandshake) runResponderNoise(
	transport connection.Transport,
	msg1WithMAC []byte,
) (serverHandshakeOutcome, error) {
	hs, err := h.newResponderState()
	if err != nil {
		return serverHandshakeOutcome{}, err
	}
	defer zeroizeLocalEphemeral(hs)

	noiseMsg := ExtractNoiseMsg(msg1WithMAC)
	_, _, _, err = hs.ReadMessage(nil, noiseMsg)
	if err != nil {
		return serverHandshakeOutcome{}, fmt.Errorf("noise: read msg1: %w", err)
	}

	clientPubKey := hs.PeerStatic()
	clientID, enabled, found := h.allowedPeers.Lookup(clientPubKey)
	if !found {
		return serverHandshakeOutcome{}, ErrUnknownPeer
	}
	if !enabled {
		return serverHandshakeOutcome{}, ErrPeerDisabled
	}

	msg2, cs1, cs2, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return serverHandshakeOutcome{}, fmt.Errorf("noise: write msg2: %w", err)
	}
	if _, err := transport.Write(msg2); err != nil {
		return serverHandshakeOutcome{}, fmt.Errorf("noise: send msg2: %w", err)
	}
	if cs1 == nil || cs2 == nil {
		return serverHandshakeOutcome{}, fmt.Errorf("noise: handshake not complete after msg2")
	}

	return serverHandshakeOutcome{
		clientID:     clientID,
		clientPubKey: clientPubKey,
		material:     extractSessionMaterial(cs1, cs2, hs.ChannelBinding()),
	}, nil
}

func (h *IKHandshake) newInitiatorState() (*noiselib.HandshakeState, error) {
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
		return nil, fmt.Errorf("noise: client handshake state: %w", err)
	}
	return hs, nil
}

func (h *IKHandshake) initiatorAttempt(
	transport connection.Transport,
	sendErrPrefix string,
	readErrPrefix string,
) (*noiselib.HandshakeState, []byte, error) {
	hs, err := h.newInitiatorState()
	if err != nil {
		return nil, nil, err
	}

	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		zeroizeLocalEphemeral(hs)
		return nil, nil, fmt.Errorf("noise: write msg1: %w", err)
	}
	msg1WithMAC, err := AppendMACs(msg1, h.peerPubKey, h.cookie)
	if err != nil {
		zeroizeLocalEphemeral(hs)
		return nil, nil, fmt.Errorf("noise: append MACs: %w", err)
	}
	msgWithVersion := PrependVersion(msg1WithMAC)
	if _, err := transport.Write(msgWithVersion); err != nil {
		zeroizeLocalEphemeral(hs)
		return nil, nil, fmt.Errorf("%s: %w", sendErrPrefix, err)
	}
	response, err := readHandshakeMessage(transport, readErrPrefix)
	if err != nil {
		zeroizeLocalEphemeral(hs)
		return nil, nil, err
	}
	return hs, response, nil
}

func (h *IKHandshake) completeInitiatorFromMsg2(
	hs *noiselib.HandshakeState,
	response []byte,
	verifyServerStatic bool,
) (sessionMaterial, error) {
	_, cs1, cs2, err := hs.ReadMessage(nil, response)
	if err != nil {
		return sessionMaterial{}, fmt.Errorf("noise: read msg2: %w", err)
	}
	if cs1 == nil || cs2 == nil {
		return sessionMaterial{}, fmt.Errorf("noise: handshake not complete after msg2")
	}
	if verifyServerStatic && !bytes.Equal(hs.PeerStatic(), h.peerPubKey) {
		return sessionMaterial{}, fmt.Errorf("noise: server static key mismatch")
	}
	return extractSessionMaterial(cs1, cs2, hs.ChannelBinding()), nil
}

func extractSessionMaterial(
	cs1, cs2 *noiselib.CipherState,
	channelBinding []byte,
) sessionMaterial {
	c2sKey := cs1.UnsafeKey()
	s2cKey := cs2.UnsafeKey()

	material := sessionMaterial{
		clientKey: make([]byte, 32),
		serverKey: make([]byte, 32),
	}
	copy(material.clientKey, c2sKey[:])
	copy(material.serverKey, s2cKey[:])
	mem.ZeroBytes(c2sKey[:])
	mem.ZeroBytes(s2cKey[:])
	copy(material.id[:], channelBinding[:32])
	return material
}

func (h *IKHandshake) applySessionMaterial(material sessionMaterial) {
	h.clientKey = material.clientKey
	h.serverKey = material.serverKey
	h.id = material.id
}

func newIKHandshakeResult(clientID int, clientPubKey []byte) *ikHandshakeResult {
	pubKeyCopy := make([]byte, len(clientPubKey))
	copy(pubKeyCopy, clientPubKey)
	return &ikHandshakeResult{
		clientID:     clientID,
		clientPubKey: pubKeyCopy,
		allowedIPs:   nil,
	}
}

func readHandshakeMessage(
	transport connection.Transport,
	errPrefix string,
) ([]byte, error) {
	buf := make([]byte, 2048)
	n, err := transport.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errPrefix, err)
	}
	return buf[:n], nil
}

func zeroizeLocalEphemeral(hs *noiselib.HandshakeState) {
	if hs == nil {
		return
	}
	localEph := hs.LocalEphemeral()
	if localEph.Private != nil {
		mem.ZeroBytes(localEph.Private)
	}
}
