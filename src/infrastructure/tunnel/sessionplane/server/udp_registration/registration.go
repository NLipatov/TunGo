package udp_registration

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"tungo/application/listeners"
	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/infrastructure/network/udp/adapters"
	"tungo/infrastructure/network/udp/queue/udp"
	"tungo/infrastructure/tunnel/session"
)

const (
	RegistrationQueueCapacity = 16
	// HandshakeTimeout bounds how long we keep a registration goroutine alive
	// in case the client stalls or disappears.
	HandshakeTimeout = 10 * time.Second
	// MaxConcurrentRegistrations limits the number of simultaneous handshakes
	// to prevent memory exhaustion from spoofed source addresses.
	MaxConcurrentRegistrations = 1000
)

// Registrar is the session-plane component responsible for turning unknown UDP peers
// into established sessions (handshake + crypto init) using a per-client packet queue.
type Registrar struct {
	ctx context.Context

	listenerConn listeners.UdpListener
	sessionRepo  session.Repository
	logger       logging.Logger

	handshakeFactory    connection.HandshakeFactory
	cryptographyFactory connection.CryptoFactory

	mu            sync.Mutex
	registrations map[netip.AddrPort]*udp.RegistrationQueue

	sendReset func(addrPort netip.AddrPort)
}

func NewRegistrar(
	ctx context.Context,
	listenerConn listeners.UdpListener,
	sessionRepo session.Repository,
	logger logging.Logger,
	handshakeFactory connection.HandshakeFactory,
	cryptographyFactory connection.CryptoFactory,
	sendReset func(addrPort netip.AddrPort),
) *Registrar {
	return &Registrar{
		ctx:                 ctx,
		listenerConn:        listenerConn,
		sessionRepo:         sessionRepo,
		logger:              logger,
		handshakeFactory:    handshakeFactory,
		cryptographyFactory: cryptographyFactory,
		registrations:       make(map[netip.AddrPort]*udp.RegistrationQueue),
		sendReset:           sendReset,
	}
}

func (r *Registrar) EnqueuePacket(addrPort netip.AddrPort, packet []byte) {
	q, isNew := r.getOrCreateRegistrationQueue(addrPort)
	if q == nil {
		// At registration capacity - silently drop to prevent DoS amplification.
		// Legitimate clients will retry; attackers waste resources.
		return
	}
	q.Enqueue(packet)
	if isNew {
		go r.RegisterClient(addrPort, q)
	}
}

func (r *Registrar) CloseAll() {
	r.mu.Lock()
	queues := make([]*udp.RegistrationQueue, 0, len(r.registrations))
	for _, q := range r.registrations {
		queues = append(queues, q)
	}
	r.registrations = make(map[netip.AddrPort]*udp.RegistrationQueue)
	r.mu.Unlock()

	for _, q := range queues {
		q.Close()
	}
}

func (r *Registrar) GetOrCreateRegistrationQueue(addrPort netip.AddrPort) (*udp.RegistrationQueue, bool) {
	return r.getOrCreateRegistrationQueue(addrPort)
}

func (r *Registrar) getOrCreateRegistrationQueue(addrPort netip.AddrPort) (*udp.RegistrationQueue, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if q, ok := r.registrations[addrPort]; ok {
		return q, false
	}

	// Enforce maximum concurrent registrations to prevent memory exhaustion
	// from spoofed source addresses.
	if len(r.registrations) >= MaxConcurrentRegistrations {
		// At capacity - reject new registration attempts.
		// Return nil queue; caller must handle gracefully.
		return nil, false
	}

	q := udp.NewRegistrationQueue(RegistrationQueueCapacity)
	r.registrations[addrPort] = q
	return q, true
}

func (r *Registrar) removeRegistrationQueue(addrPort netip.AddrPort) {
	r.mu.Lock()
	q, ok := r.registrations[addrPort]
	if ok {
		delete(r.registrations, addrPort)
	}
	r.mu.Unlock()

	if ok {
		q.Close()
	}
}

func (r *Registrar) RegisterClient(addrPort netip.AddrPort, queue *udp.RegistrationQueue) {
	defer r.removeRegistrationQueue(addrPort)

	ctx, cancel := context.WithTimeout(r.ctx, HandshakeTimeout)
	defer cancel()

	go func() {
		<-ctx.Done()
		queue.Close()
	}()

	h := r.handshakeFactory.NewHandshake()

	// Transport reads from client's RegistrationQueue (fed by dataplane EnqueuePacket)
	// and writes responses to the shared UDP socket.
	regTransport := adapters.NewRegistrationTransport(r.listenerConn, addrPort, queue)

	internalIP, handshakeErr := h.ServerSideHandshake(regTransport)
	if handshakeErr != nil {
		r.logger.Printf("host %v failed registration: %v", addrPort.Addr().AsSlice(), handshakeErr)
		r.sendReset(addrPort)
		return
	}

	cryptoSession, controller, cryptoSessionErr := r.cryptographyFactory.FromHandshake(h, true)
	if cryptoSessionErr != nil {
		r.logger.Printf("failed to init crypto session for %v: %v", addrPort.Addr().AsSlice(), cryptoSessionErr)
		r.sendReset(addrPort)
		return
	}

	// Extract authentication info from IK handshake result if available
	var clientPubKey []byte
	var allowedIPs []netip.Prefix
	if hwr, ok := h.(connection.HandshakeWithResult); ok {
		if result := hwr.Result(); result != nil {
			clientPubKey = result.ClientPubKey()
			allowedIPs = result.AllowedIPs()
		}
	}

	sess := session.NewSessionWithAuth(cryptoSession, controller, internalIP, addrPort, clientPubKey, allowedIPs)
	egress := connection.NewDefaultEgress(regTransport, cryptoSession)
	peer := session.NewPeer(sess, egress)
	r.sessionRepo.Add(peer)
	r.logger.Printf("UDP: %v registered as: %v", addrPort.Addr(), internalIP)
}

// Registrations exposes the internal registrations map for testing.
func (r *Registrar) Registrations() map[netip.AddrPort]*udp.RegistrationQueue {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registrations
}
