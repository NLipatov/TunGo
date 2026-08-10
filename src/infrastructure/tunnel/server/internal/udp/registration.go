package udp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"tungo/application/listeners"
	"tungo/application/network/ip"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/network/udp/adapters"
	"tungo/infrastructure/tunnel/internal/rekey"
	"tungo/infrastructure/tunnel/server/internal/session"
)

type handshake interface {
	chacha20.KeyMaterial
	ServerSideHandshake(io.ReadWriter) (int, error)
}

type authenticatedHandshake interface {
	ClientPubKey() []byte
	AllowedIPs() []netip.Prefix
}

type rekeyV2Handshake interface {
	Supports(noise.Capability) bool
	RespondRekeyV2(prologue, msg1 []byte) (msg2, c2s, s2c []byte, err error)
}

type crypto interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

type epochController interface {
	ReadyForRekey() bool
	SendEpoch() uint16
	StartRekey(c2s, s2c []byte) (uint16, error)
	ActivateSendEpoch(uint16)
	ObservePeerEpoch(uint16)
	CurrentKeys() (clientToServer, serverToClient []byte)
}

type newHandshake func() handshake
type newCrypto func(chacha20.KeyMaterial, bool) (crypto, epochController, error)

const (
	registrationQueueCapacity = 16
	// handshakeTimeout bounds how long we keep a registration goroutine alive
	// in case the client stalls or disappears.
	handshakeTimeout = 10 * time.Second
	// maxConcurrentRegistrations limits the number of simultaneous handshakes
	// to prevent memory exhaustion from spoofed source addresses.
	maxConcurrentRegistrations = 1000
)

// registrar turns unknown UDP peers into established sessions using a
// per-client packet queue.
type registrar struct {
	ctx context.Context

	listenerConn listeners.UdpListener
	sessionRepo  udpRegistrationRepo

	newHandshake newHandshake
	newCrypto    newCrypto

	interfaceSubnet netip.Prefix
	ipv6Subnet      netip.Prefix

	mu            sync.Mutex
	registrations map[netip.AddrPort]*registrationQueue
}

type udpRegistrationRepo interface {
	Add(*session.Peer)
	Delete(*session.Peer)
	GetByInternalAddrPort(netip.Addr) (*session.Peer, error)
}

func newRegistrar(
	ctx context.Context,
	listenerConn listeners.UdpListener,
	sessionRepo udpRegistrationRepo,
	newHandshake newHandshake,
	newCrypto newCrypto,
	interfaceSubnet netip.Prefix,
	ipv6Subnet netip.Prefix,
) *registrar {
	return &registrar{
		ctx:             ctx,
		listenerConn:    listenerConn,
		sessionRepo:     sessionRepo,
		newHandshake:    newHandshake,
		newCrypto:       newCrypto,
		interfaceSubnet: interfaceSubnet,
		ipv6Subnet:      ipv6Subnet,
		registrations:   make(map[netip.AddrPort]*registrationQueue),
	}
}

func (r *registrar) enqueuePacket(addrPort netip.AddrPort, packet []byte) {
	q, isNew := r.getOrCreateRegistrationQueue(addrPort)
	if q == nil {
		// At registration capacity - silently drop to prevent DoS amplification.
		// Legitimate clients will retry; attackers waste resources.
		return
	}
	q.enqueue(packet)
	if isNew {
		go r.registerClient(addrPort, q)
	}
}

func (r *registrar) closeAll() {
	r.mu.Lock()
	queues := make([]*registrationQueue, 0, len(r.registrations))
	for _, q := range r.registrations {
		queues = append(queues, q)
	}
	r.registrations = make(map[netip.AddrPort]*registrationQueue)
	r.mu.Unlock()

	for _, q := range queues {
		q.Close()
	}
}

func (r *registrar) getOrCreateRegistrationQueue(addrPort netip.AddrPort) (*registrationQueue, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if q, ok := r.registrations[addrPort]; ok {
		return q, false
	}

	// Enforce maximum concurrent registrations to prevent memory exhaustion
	// from spoofed source addresses.
	if len(r.registrations) >= maxConcurrentRegistrations {
		// At capacity - reject new registration attempts.
		// Return nil queue; caller must handle gracefully.
		return nil, false
	}

	q := newRegistrationQueue(registrationQueueCapacity)
	r.registrations[addrPort] = q
	return q, true
}

func (r *registrar) removeRegistrationQueue(addrPort netip.AddrPort) {
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

func (r *registrar) registerClient(addrPort netip.AddrPort, queue *registrationQueue) {
	defer r.removeRegistrationQueue(addrPort)

	ctx, cancel := context.WithTimeout(r.ctx, handshakeTimeout)
	defer cancel()

	go func() {
		<-ctx.Done()
		queue.Close()
	}()

	// Transport reads packets queued by the UDP receive loop during registration.
	// and writes responses to the shared UDP socket.
	regTransport := adapters.NewRegistrationTransport(r.listenerConn, addrPort, queue)

	var h handshake
	var clientID int
	for attempt := 0; ; attempt++ {
		h = r.newHandshake()
		var handshakeErr error
		clientID, handshakeErr = h.ServerSideHandshake(regTransport)
		if handshakeErr == nil {
			break
		}
		if errors.Is(handshakeErr, noise.ErrCookieRequired) && attempt == 0 {
			slog.Warn("UDP cookie sent, awaiting retry", "client", addrPort.Addr().AsSlice())
			continue
		}
		slog.Warn("UDP host failed registration", "client", addrPort.Addr().AsSlice(), "err", handshakeErr)
		return
	}

	internalIP, allocErr := ip.AllocateClientIP(r.interfaceSubnet, clientID)
	if allocErr != nil {
		slog.Error("UDP host IP allocation failed", "client", addrPort.Addr().AsSlice(), "err", allocErr)
		return
	}

	cryptoSession, epochController, cryptoSessionErr := r.newCrypto(h, true)
	if cryptoSessionErr != nil {
		slog.Error("failed to init UDP crypto session", "client", addrPort.Addr().AsSlice(), "err", cryptoSessionErr)
		return
	}
	var rekeyCoordinator *rekey.ServerRekeyCoordinator
	if epochController != nil {
		var rehandshake rekeyV2Handshake
		if capable, ok := h.(rekeyV2Handshake); ok && capable.Supports(noise.CapabilityRekeyV2) {
			rehandshake = capable
		}
		rekeyCoordinator = rekey.NewServerRekeyCoordinator(epochController, rehandshake)
	}

	// Extract authentication info from IK handshake result if available
	var clientPubKey []byte
	var allowedIPs []netip.Prefix
	if authenticated, ok := h.(authenticatedHandshake); ok {
		clientPubKey = authenticated.ClientPubKey()
		allowedIPs = authenticated.AllowedIPs()
	}

	// Add IPv6 address to allowedIPs for dual-stack support
	if r.ipv6Subnet.IsValid() {
		ipv6Addr, ipv6Err := ip.AllocateClientIP(r.ipv6Subnet, clientID)
		if ipv6Err == nil {
			allowedIPs = append(allowedIPs, netip.PrefixFrom(ipv6Addr, 128))
		}
	}

	// Evict stale session for the same internal IP (e.g. client reconnect after NAT rebinding).
	existingPeer, getErr := r.sessionRepo.GetByInternalAddrPort(internalIP)
	if getErr == nil {
		r.sessionRepo.Delete(existingPeer)
		slog.Info("UDP replacing existing session", "internal_ip", internalIP)
	}

	peer := session.NewPeerWithAuth(
		cryptoSession, rekeyCoordinator, internalIP, addrPort, clientPubKey, allowedIPs, regTransport,
	)
	r.sessionRepo.Add(peer)
	slog.Info("UDP client registered", "client", addrPort.Addr(), "internal_ip", internalIP)
}
