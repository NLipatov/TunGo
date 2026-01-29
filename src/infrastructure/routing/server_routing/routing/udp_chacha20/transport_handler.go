package udp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sync"
	"time"
	"tungo/application/listeners"
	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/network/udp/adapters"
	"tungo/infrastructure/network/udp/queue/udp"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"
)

const (
	RegistrationQueueCapacity = 16
	// HandshakeTimeout bounds how long we keep a registration goroutine alive
	// in case the client stalls or disappears.
	HandshakeTimeout = 10 * time.Second
)

// TransportHandler handles incoming UDP packets, routes them either to
// existing sessions or to the registration pipeline, and writes decrypted
// payloads into the TUN device.
type TransportHandler struct {
	ctx                 context.Context
	settings            settings.Settings
	writer              io.Writer
	sessionManager      repository.SessionRepository[connection.Session]
	logger              logging.Logger
	listenerConn        listeners.UdpListener
	handshakeFactory    connection.HandshakeFactory
	cryptographyFactory connection.CryptoFactory
	crypto              handshake.Crypto
	// registrations holds per-client registration queues for clients that are
	// currently performing a handshake.
	regMu         sync.Mutex
	registrations map[netip.AddrPort]*udp.RegistrationQueue
	sp            servicePacketHandler
}

// NewTransportHandler constructs a new UDP transport handler.
func NewTransportHandler(
	ctx context.Context,
	settings settings.Settings,
	writer io.Writer,
	listenerConn listeners.UdpListener,
	sessionManager repository.SessionRepository[connection.Session],
	logger logging.Logger,
	handshakeFactory connection.HandshakeFactory,
	cryptographyFactory connection.CryptoFactory,
) transport.Handler {
	crypto := &handshake.DefaultCrypto{}
	return &TransportHandler{
		ctx:                 ctx,
		settings:            settings,
		writer:              writer,
		sessionManager:      sessionManager,
		logger:              logger,
		listenerConn:        listenerConn,
		handshakeFactory:    handshakeFactory,
		cryptographyFactory: cryptographyFactory,
		crypto:              crypto,
		registrations:       make(map[netip.AddrPort]*udp.RegistrationQueue),
		sp:                  newServicePacketHandler(crypto),
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
			t.closeAllRegistrations()
			return nil
		default:
			n, _, _, clientAddr, readFromUdpErr := t.listenerConn.ReadMsgUDPAddrPort(buffer[:], oobBuf[:])
			if readFromUdpErr != nil {
				if t.ctx.Err() != nil {
					t.closeAllRegistrations()
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
	session, sessionLookupErr := t.sessionManager.GetByExternalAddrPort(addrPort)
	if sessionLookupErr == nil && session.ExternalAddrPort() == addrPort {
		rekeyCtrl := session.RekeyController()
		decrypted, decryptionErr := session.Crypto().Decrypt(packet)
		if decryptionErr != nil {
			// Drop: untrusted UDP input can be garbage / attacker-driven.
			return nil
		}
		if rekeyCtrl != nil {
			// Data was successfully decrypted with epoch.
			// Epoch can now be used to encrypt. Allow to encrypt with this epoch by promoting.
			rekeyCtrl.ActivateSendEpoch(binary.BigEndian.Uint16(packet[:2]))
			rekeyCtrl.AbortPendingIfExpired(time.Now())
			// If service_packet packet - handle it.
			if handled, err := t.sp.Handle(decrypted, session, rekeyCtrl); handled {
				return err
			}
		}
		// Pass it to TUN
		_, err := t.writer.Write(decrypted)
		if err != nil {
			return fmt.Errorf("failed to write to TUN: %v", err)
		}
		return nil
	}

	// No existing session: route into registration queue.
	q, isNew := t.getOrCreateRegistrationQueue(addrPort)
	q.Enqueue(packet)

	if isNew {
		// Run handshake in a separate goroutine so HandleTransport keeps
		// serving traffic for already registered clients.
		go t.registerClient(addrPort, q)
	}

	return nil
}

// getOrCreateRegistrationQueue returns an existing RegistrationQueue for
// addrPort or creates a new one. The boolean indicates whether it was newly
// created.
func (t *TransportHandler) getOrCreateRegistrationQueue(
	addrPort netip.AddrPort,
) (*udp.RegistrationQueue, bool) {
	t.regMu.Lock()
	defer t.regMu.Unlock()

	if q, ok := t.registrations[addrPort]; ok {
		return q, false
	}

	q := udp.NewRegistrationQueue(RegistrationQueueCapacity)
	t.registrations[addrPort] = q
	return q, true
}

// removeRegistrationQueue removes and closes the RegistrationQueue for addrPort
// if it exists.
func (t *TransportHandler) removeRegistrationQueue(addrPort netip.AddrPort) {
	t.regMu.Lock()
	q, ok := t.registrations[addrPort]
	if ok {
		delete(t.registrations, addrPort)
	}
	t.regMu.Unlock()

	if ok {
		q.Close()
	}
}

// closeAllRegistrations force-closes all active registration queues.
// This is useful on handler shutdown to unblock any pending handshakes.
func (t *TransportHandler) closeAllRegistrations() {
	t.regMu.Lock()
	queues := make([]*udp.RegistrationQueue, 0, len(t.registrations))
	for _, q := range t.registrations {
		queues = append(queues, q)
	}
	t.registrations = make(map[netip.AddrPort]*udp.RegistrationQueue)
	t.regMu.Unlock()
	for _, q := range queues {
		q.Close()
	}
}

// registerClient performs server-side handshake for a single client using
// a per-client RegistrationQueue as the source of incoming packets.
//
// The lifetime of this goroutine is bounded by a context derived from
// TransportHandler.ctx with a timeout. On ctx cancellation/timeout, the queue
// is closed, which unblocks ReadInto and allows this goroutine to exit.
func (t *TransportHandler) registerClient(
	addrPort netip.AddrPort,
	queue *udp.RegistrationQueue,
) {
	// Ensure we always remove the registration entry and close the queue.
	defer t.removeRegistrationQueue(addrPort)

	// Derive a context that bounds handshake lifetime. It reacts both to
	// server shutdown (t.ctx.Done) and to registration timeout.
	ctx, cancel := context.WithTimeout(t.ctx, HandshakeTimeout)
	defer cancel()

	// Watch for context cancellation and close the queue to unblock any
	// pending ReadInto calls.
	go func() {
		<-ctx.Done()
		queue.Close()
	}()

	h := t.handshakeFactory.NewHandshake()

	// Transport reads from client's RegistrationQueue (fed by handlePacket)
	// and writes responses to the shared UDP socket.
	regTransport := adapters.NewRegistrationTransport(t.listenerConn, addrPort, queue)
	adapter := adapters.NewInitialDataAdapter(regTransport, nil)

	internalIP, handshakeErr := h.ServerSideHandshake(adapter)
	if handshakeErr != nil {
		t.logger.Printf("host %v failed registration: %v", addrPort.Addr().AsSlice(), handshakeErr)
		t.sendSessionReset(addrPort)
		return
	}

	cryptoSession, controller, cryptoSessionErr := t.cryptographyFactory.FromHandshake(h, true)
	if cryptoSessionErr != nil {
		t.logger.Printf("failed to init crypto session for %v: %v", addrPort.Addr().AsSlice(), cryptoSessionErr)
		t.sendSessionReset(addrPort)
		return
	}

	intIp, intIpOk := netip.AddrFromSlice(internalIP)
	if !intIpOk {
		t.logger.Printf("failed to parse internal IP: %v", internalIP)
		t.sendSessionReset(addrPort)
		return
	}

	t.sessionManager.Add(NewSession(adapter, cryptoSession, controller, intIp, addrPort))
	t.logger.Printf("UDP: %v registered as: %v", addrPort.Addr(), internalIP)
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
