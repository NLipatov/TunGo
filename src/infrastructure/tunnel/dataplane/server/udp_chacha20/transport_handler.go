package udp_chacha20

import (
	"context"
	"io"
	"net/netip"
	"tungo/application/listeners"
	"tungo/application/logging"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20"
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
// - Fast path: route-id lookup in O(1), then decrypts and forwards to TUN.
// - Unknown peers are delegated to registration pipeline.
func (t *TransportHandler) handlePacket(
	addrPort netip.AddrPort,
	packet []byte,
) error {
	if len(packet) < chacha20.UDPRouteIDLength {
		t.logger.Printf("packet too short for route id from %v: %d bytes", addrPort, len(packet))
	} else {
		if peer, ok := t.getPeerByRouteID(packet); ok {
			return t.handleEstablished(addrPort, peer, packet)
		}
	}

	// No existing session: route into registration queue.
	if t.registrar != nil {
		t.registrar.EnqueuePacket(addrPort, packet)
	}

	return nil
}

func (t *TransportHandler) getPeerByRouteID(packet []byte) (*session.Peer, bool) {
	routeID, ok := chacha20.ReadUDPRouteID(packet)
	if !ok {
		return nil, false
	}
	peer, err := t.sessionManager.GetByRouteID(routeID)
	if err != nil {
		return nil, false
	}
	return peer, true
}

func (t *TransportHandler) handleEstablished(addrPort netip.AddrPort, peer *session.Peer, packet []byte) error {
	if t.dp == nil || peer == nil || peer.IsClosed() {
		return nil
	}

	decrypted, ok := tryDecryptSafe(peer, packet)
	if !ok {
		return nil
	}

	if peer.ExternalAddrPort() != addrPort {
		t.sessionManager.UpdateExternalAddr(peer, addrPort)
	}

	return t.dp.handleDecrypted(peer, packet, decrypted)
}

// tryDecryptSafe attempts decryption under the peer's crypto read lock,
// preventing the TOCTOU race where crypto could be zeroed concurrently.
// Used by the established session path.
func tryDecryptSafe(peer *session.Peer, data []byte) ([]byte, bool) {
	if !peer.CryptoRLock() {
		return nil, false
	}
	defer peer.CryptoRUnlock()
	result, err := peer.Crypto().Decrypt(data)
	if err != nil {
		return nil, false
	}
	return result, true
}
