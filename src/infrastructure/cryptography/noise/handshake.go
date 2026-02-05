package noise

import (
	"bytes"
	"fmt"
	"net"
	"tungo/application/network/connection"
	"tungo/infrastructure/settings"

	noiselib "github.com/flynn/noise"
)

var cipherSuite = noiselib.NewCipherSuite(noiselib.DH25519, noiselib.CipherChaChaPoly, noiselib.HashSHA256)

// ZeroBytes overwrites a byte slice with zeros.
func ZeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// NoiseHandshake implements connection.Handshake using Noise XX.
type NoiseHandshake struct {
	id        [32]byte
	clientKey []byte
	serverKey []byte

	// Static keypair: server has both, client has only peer's public key.
	staticPublicKey  []byte
	staticPrivateKey []byte
}

// NewNoiseHandshake creates a new Noise XX handshake.
// Server: staticPublicKey + staticPrivateKey (X25519 keys derived from Ed25519 seeds).
// Client: only peerStaticPublicKey (server's public key).
func NewNoiseHandshake(staticPublicKey, staticPrivateKey []byte) *NoiseHandshake {
	return &NoiseHandshake{
		staticPublicKey:  staticPublicKey,
		staticPrivateKey: staticPrivateKey,
	}
}

func (h *NoiseHandshake) Id() [32]byte              { return h.id }
func (h *NoiseHandshake) KeyClientToServer() []byte { return h.clientKey }
func (h *NoiseHandshake) KeyServerToClient() []byte { return h.serverKey }

// ServerSideHandshake performs Noise XX as responder.
// Returns the client's internal IP and the same transport passed in.
func (h *NoiseHandshake) ServerSideHandshake(
	transport connection.Transport,
) (net.IP, error) {
	staticKey := noiselib.DHKey{
		Private: h.staticPrivateKey,
		Public:  h.staticPublicKey,
	}

	hs, err := noiselib.NewHandshakeState(noiselib.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noiselib.HandshakeXX,
		Initiator:     false,
		StaticKeypair: staticKey,
	})
	if err != nil {
		return nil, fmt.Errorf("noise: server handshake state: %w", err)
	}

	// --- Message 1: Read client's ephemeral (e) ---
	msg1Buf := make([]byte, 2048)
	n, err := transport.Read(msg1Buf)
	if err != nil {
		return nil, fmt.Errorf("noise: read msg1: %w", err)
	}

	_, _, _, err = hs.ReadMessage(nil, msg1Buf[:n])
	if err != nil {
		return nil, fmt.Errorf("noise: read msg1 payload: %w", err)
	}

	// --- Message 2: Write server's e + s + DHEE + DHES ---
	msg2, cs1, cs2, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("noise: write msg2: %w", err)
	}

	if _, err := transport.Write(msg2); err != nil {
		return nil, fmt.Errorf("noise: send msg2: %w", err)
	}

	// Zero the server's ephemeral private key after msg2. WriteMessage generates
	// the ephemeral lazily via the "e" token, so LocalEphemeral() is only valid
	// after this point. The returned slice shares the library's internal backing
	// array, so zeroing it scrubs the ephemeral from heap memory.
	// NOTE: ReadMessage(msg3) still needs e.Private for the "se" DH token,
	// so defer ensures zeroing happens on function exit, not before msg3.
	localEph := hs.LocalEphemeral()
	defer ZeroBytes(localEph.Private)

	// XX has 3 messages. cs1/cs2 should be nil after msg2 (not the last message).
	if cs1 != nil || cs2 != nil {
		return nil, fmt.Errorf("noise: unexpected cipher states after msg2")
	}

	// --- Message 3: Read client's s + DHSE + payload(IP) ---
	msg3Buf := make([]byte, 2048)
	n3, err := transport.Read(msg3Buf)
	if err != nil {
		return nil, fmt.Errorf("noise: read msg3: %w", err)
	}

	payload3, cs1, cs2, err := hs.ReadMessage(nil, msg3Buf[:n3])
	if err != nil {
		return nil, fmt.Errorf("noise: read msg3 noise: %w", err)
	}
	if cs1 == nil || cs2 == nil {
		return nil, fmt.Errorf("noise: handshake not complete after msg3")
	}

	// payload3 contains the client's internal IP.
	clientIP := net.IP(payload3)
	if clientIP == nil || (len(clientIP) != net.IPv4len && len(clientIP) != net.IPv6len) {
		return nil, fmt.Errorf("noise: invalid client IP in msg3 payload")
	}

	// Extract session keys from Noise cipher states.
	c2sKey := cs1.UnsafeKey()
	s2cKey := cs2.UnsafeKey()

	h.clientKey = make([]byte, 32)
	copy(h.clientKey, c2sKey[:])
	h.serverKey = make([]byte, 32)
	copy(h.serverKey, s2cKey[:])
	ZeroBytes(c2sKey[:])
	ZeroBytes(s2cKey[:])

	// Session ID from channel binding.
	cb := hs.ChannelBinding()
	copy(h.id[:], cb[:32])

	return clientIP, nil
}

// ClientSideHandshake performs Noise XX as initiator.
func (h *NoiseHandshake) ClientSideHandshake(
	transport connection.Transport,
	s settings.Settings,
) error {
	// Client generates an ephemeral static keypair for this handshake.
	clientStatic, err := cipherSuite.GenerateKeypair(nil)
	if err != nil {
		return fmt.Errorf("noise: generate client static: %w", err)
	}
	defer ZeroBytes(clientStatic.Private)

	hs, err := noiselib.NewHandshakeState(noiselib.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noiselib.HandshakeXX,
		Initiator:     true,
		StaticKeypair: clientStatic,
	})
	if err != nil {
		return fmt.Errorf("noise: client handshake state: %w", err)
	}

	// --- Message 1: Write ephemeral key (e) ---
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return fmt.Errorf("noise: write msg1: %w", err)
	}

	// Zero the client's ephemeral private key on exit. The ephemeral is generated
	// lazily during WriteMessage for msg1, so LocalEphemeral() is valid here.
	// It must remain available for DH operations in msg2, hence defer.
	localEph := hs.LocalEphemeral()
	defer ZeroBytes(localEph.Private)

	if _, err := transport.Write(msg1); err != nil {
		return fmt.Errorf("noise: send msg1: %w", err)
	}

	// --- Message 2: Read server's e + s + DHEE + DHES ---
	msg2Buf := make([]byte, 2048)
	n2, err := transport.Read(msg2Buf)
	if err != nil {
		return fmt.Errorf("noise: read msg2: %w", err)
	}

	_, _, _, err = hs.ReadMessage(nil, msg2Buf[:n2])
	if err != nil {
		return fmt.Errorf("noise: read msg2 noise: %w", err)
	}

	// Verify server's static key.
	peerStatic := hs.PeerStatic()
	if !bytes.Equal(peerStatic, h.staticPublicKey) {
		return fmt.Errorf("noise: server static key mismatch")
	}

	// --- Message 3: Write s + DHSE + payload(IP) ---
	clientIP := net.ParseIP(s.InterfaceAddress)
	if clientIP == nil {
		return fmt.Errorf("noise: invalid client IP: %s", s.InterfaceAddress)
	}
	ip4 := clientIP.To4()
	if ip4 != nil {
		clientIP = ip4
	}

	msg3, cs1, cs2, err := hs.WriteMessage(nil, clientIP)
	if err != nil {
		return fmt.Errorf("noise: write msg3: %w", err)
	}
	if cs1 == nil || cs2 == nil {
		return fmt.Errorf("noise: handshake not complete after msg3")
	}

	if _, err := transport.Write(msg3); err != nil {
		return fmt.Errorf("noise: send msg3: %w", err)
	}

	// Extract session keys.
	c2sKey := cs1.UnsafeKey()
	s2cKey := cs2.UnsafeKey()

	h.clientKey = make([]byte, 32)
	copy(h.clientKey, c2sKey[:])
	h.serverKey = make([]byte, 32)
	copy(h.serverKey, s2cKey[:])
	ZeroBytes(c2sKey[:])
	ZeroBytes(s2cKey[:])

	cb := hs.ChannelBinding()
	copy(h.id[:], cb[:32])

	return nil
}
