package udp_chacha20

import (
	"context"
	"io"
	"net/netip"
	"tungo/application/listeners"
	"tungo/application/logging"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"
	"tungo/infrastructure/tunnel/sessionplane/server/udp_registration"
)

// TransportHandler is the UDP dataplane ingress:
// - reads ciphertext packets from UDP socket
// - decrypts with an established session
// - dispatches control-plane packets
// - writes data-plane payloads to TUN
//
// For unknown clients, it delegates to session-plane registration via the injected registrar.
type TransportHandler struct {
	ctx            context.Context
	settings       settings.Settings
	writer         io.Writer
	sessionManager session.Repository
	logger         logging.Logger
	listenerConn   listeners.UdpListener
	// Session-plane: registration and handshake tracking for not-yet-established sessions.
	registrar *udp_registration.Registrar
	// Dataplane worker for established sessions.
	dp *udpDataplaneWorker
}

// NewTransportHandler constructs a new UDP transport handler.
func NewTransportHandler(
	ctx context.Context,
	settings settings.Settings,
	writer io.Writer,
	listenerConn listeners.UdpListener,
	sessionManager session.Repository,
	logger logging.Logger,
	registrar *udp_registration.Registrar,
) transport.Handler {
	crypto := &primitives.DefaultKeyDeriver{}
	cp := newServicePacketHandler(crypto)
	dp := newUdpDataplaneWorker(writer, cp)
	return &TransportHandler{
		ctx:            ctx,
		settings:       settings,
		writer:         writer,
		sessionManager: sessionManager,
		logger:         logger,
		listenerConn:   listenerConn,
		registrar:      registrar,
		dp:             dp,
	}
}

// HandleTransport runs the main UDP read loop and dispatches packets either
// to existing sessions or to the registration pipeline.
func (t *TransportHandler) HandleTransport() error {
	defer func(conn listeners.UdpListener) {
		_ = conn.Close()
	}(t.listenerConn)

	t.logger.Printf("server listening on port %d (UDP)", t.settings.Port)

	// Start idle session reaper if the repository supports it.
	if reaper, ok := t.sessionManager.(session.IdleReaper); ok {
		go session.RunIdleReaperLoop(t.ctx, reaper, settings.ServerIdleTimeout, settings.IdleReaperInterval, t.logger)
	}

	// Size socket buffers for burst absorption under high throughput.
	_ = t.listenerConn.SetReadBuffer(4 * 1024 * 1024)
	_ = t.listenerConn.SetWriteBuffer(4 * 1024 * 1024)

	go func() {
		<-t.ctx.Done()
		_ = t.listenerConn.Close()
	}()

	var buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	var oobBuf [1024]byte

	for {
		select {
		case <-t.ctx.Done():
			// Optionally, aggressively close all registration queues here as
			// an extra safety net:
			if t.registrar != nil {
				t.registrar.CloseAll()
			}
			return nil
		default:
			n, _, _, clientAddr, readFromUdpErr := t.listenerConn.ReadMsgUDPAddrPort(buffer[:], oobBuf[:])
			if readFromUdpErr != nil {
				if t.ctx.Err() != nil {
					if t.registrar != nil {
						t.registrar.CloseAll()
					}
					return t.ctx.Err()
				}
				t.logger.Printf("failed to read from UDP: %s", readFromUdpErr)
				continue
			}
			if n == 0 {
				t.logger.Printf("packet dropped: empty packet from %v", clientAddr.String())
				continue
			}

			// Pass the slice view into the handler. The handler will copy it
			// only when needed (for registration).
			if err := t.handlePacket(clientAddr, buffer[:n]); err != nil {
				t.logger.Printf("failed to handle packet: %s", err)
			}
		}
	}
}

// handlePacket processes a UDP packet from addrPort.
// - If a session exists, decrypts and forwards the packet to the TUN device.
// - If the address is unknown but trial decryption succeeds (NAT roaming),
//   updates the peer's address and processes the packet.
// - Otherwise, enqueues the packet into the registration pipeline.
func (t *TransportHandler) handlePacket(
	addrPort netip.AddrPort,
	packet []byte,
) error {
	if len(packet) < 2 {
		t.logger.Printf("packet too short for epoch from %v: %d bytes", addrPort, len(packet))
		return nil
	}
	// Fast path: existing session.
	// SECURITY: Check IsClosed() to handle TOCTOU race with Delete.
	// The peer might be marked for deletion between lookup and use.
	peer, sessionLookupErr := t.sessionManager.GetByExternalAddrPort(addrPort)
	if sessionLookupErr == nil && !peer.IsClosed() && peer.ExternalAddrPort() == addrPort {
		if t.dp == nil {
			// Should not happen; keep behavior safe.
			return nil
		}
		return t.dp.HandleEstablished(peer, packet)
	}

	// Trial decryption: the client may have roamed to a new NAT address.
	if t.dp != nil && t.tryRoaming(addrPort, packet) {
		return nil
	}

	// No existing session: route into registration queue.
	if t.registrar != nil {
		t.registrar.EnqueuePacket(addrPort, packet)
	}

	return nil
}

// tryRoaming attempts trial decryption against all existing sessions.
// If a session successfully decrypts the packet, the client has roamed —
// update its external address and process the packet.
//
// SAFETY: Decrypt on a wrong session fails at the AEAD auth tag check and
// does not mutate any state. We decrypt a copy because Open() modifies the
// buffer in-place.
func (t *TransportHandler) tryRoaming(newAddr netip.AddrPort, packet []byte) bool {
	peers := t.sessionManager.AllPeers()
	for _, peer := range peers {
		if peer.IsClosed() {
			continue
		}
		// Decrypt a copy — AEAD Open() overwrites ciphertext in-place.
		trial := make([]byte, len(packet))
		copy(trial, packet)

		decrypted, err := peer.Crypto().Decrypt(trial)
		if err != nil {
			continue
		}

		// Decryption succeeded — this client has roamed.
		t.sessionManager.UpdateExternalAddr(peer, newAddr)
		// Process via the shared post-decrypt path using the original rawPacket
		// (needed for epoch extraction) and the decrypted payload.
		_ = t.dp.handleDecrypted(peer, packet, decrypted)
		return true
	}
	return false
}
