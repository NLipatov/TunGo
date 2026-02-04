package udp_chacha20

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"tungo/application/listeners"
	"tungo/application/logging"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
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

	t.logger.Printf("server listening on port %s (UDP)", t.settings.Port)

	// Ensure buffers are reasonably sized for high throughput.
	_ = t.listenerConn.SetReadBuffer(65536)
	_ = t.listenerConn.SetWriteBuffer(65536)

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
				if errors.Is(err, rekey.ErrEpochExhausted) {
					t.sendSessionReset(clientAddr)
				}
			}
		}
	}
}

// handlePacket processes a UDP packet from addrPort.
// - If a session exists, decrypts and forwards the packet to the TUN device.
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
	peer, sessionLookupErr := t.sessionManager.GetByExternalAddrPort(addrPort)
	if sessionLookupErr == nil && peer.ExternalAddrPort() == addrPort {
		if t.dp == nil {
			// Should not happen; keep behavior safe.
			return nil
		}
		return t.dp.HandleEstablished(peer, packet)
	}

	// No existing session: route into registration queue.
	if t.registrar != nil {
		t.registrar.EnqueuePacket(addrPort, packet)
	}

	return nil
}

// sendSessionReset sends a SessionReset service_packet packet to the given client.
func (t *TransportHandler) sendSessionReset(addrPort netip.AddrPort) {
	servicePacketBuffer := make([]byte, 3)
	servicePacketPayload, err := service_packet.EncodeLegacyHeader(service_packet.SessionReset, servicePacketBuffer)
	if err != nil {
		t.logger.Printf("failed to encode legacy session reset service_packet packet: %v", err)
		return
	}
	_, _ = t.listenerConn.WriteToUDPAddrPort(servicePacketPayload, addrPort)
}
