package udp_chacha20

import (
	"context"
	"net/netip"

	"tungo/infrastructure/network/udp/adapters"
	"tungo/infrastructure/network/udp/queue/udp"
)

// Session-plane: registration + handshake for clients without an established session.

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
