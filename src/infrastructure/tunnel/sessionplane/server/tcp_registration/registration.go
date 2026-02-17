package tcp_registration

import (
	"errors"
	"fmt"
	"net"
	"net/netip"

	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/tcp/adapters"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"
)

// Registrar turns an untrusted net.Conn into an established Peer
// (handshake + crypto init + session repo add).
type Registrar struct {
	logger              logging.Logger
	handshakeFactory    connection.HandshakeFactory
	cryptographyFactory connection.CryptoFactory
	sessionManager      tcpRegistrationRepo
	interfaceSubnet     netip.Prefix
	ipv6Subnet          netip.Prefix
}

type tcpRegistrationRepo interface {
	session.PeerStore
	session.InternalLookup
}

func NewRegistrar(
	logger logging.Logger,
	handshakeFactory connection.HandshakeFactory,
	cryptographyFactory connection.CryptoFactory,
	sessionManager tcpRegistrationRepo,
	interfaceSubnet netip.Prefix,
	ipv6Subnet netip.Prefix,
) *Registrar {
	return &Registrar{
		logger:              logger,
		handshakeFactory:    handshakeFactory,
		cryptographyFactory: cryptographyFactory,
		sessionManager:      sessionManager,
		interfaceSubnet:     interfaceSubnet,
		ipv6Subnet:          ipv6Subnet,
	}
}

// RegisterClient performs the handshake/crypto handshake on conn,
// creates a Peer, adds it to the repository, and returns it alongside
// the framing transport. The caller is responsible for driving the
// dataplane loop using the returned peer and transport.
func (r *Registrar) RegisterClient(conn net.Conn) (*session.Peer, connection.Transport, error) {
	r.logger.Printf("TCP: %s connected", conn.RemoteAddr())

	// Extract remote address early — needed for cookie IP binding during
	// the handshake (DoS protection) and later for session tracking.
	addr := conn.RemoteAddr()
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("invalid remote address type: %T", addr)
	}

	// Enable OS-level TCP keepalive for dead connection detection.
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(settings.ServerIdleTimeout)
	}

	// Wrap with read deadline so the server detects dead clients at the
	// application level (no data within ServerIdleTimeout → connection closed).
	deadlineConn := adapters.NewReadDeadlineTransport(conn, settings.ServerIdleTimeout)

	// Attach remote address so the handshake can extract the client IP
	// for cookie binding through the LengthPrefixFramingAdapter chain.
	addrConn := adapters.NewRemoteAddrTransport(deadlineConn, tcpAddr.AddrPort())

	framingAdapter, fErr := adapters.NewLengthPrefixFramingAdapter(addrConn, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
	if fErr != nil {
		_ = conn.Close() // Prevent socket leak on framing adapter failure
		return nil, nil, fErr
	}
	var h connection.Handshake
	var clientID int
	for attempt := 0; ; attempt++ {
		h = r.handshakeFactory.NewHandshake()
		var handshakeErr error
		clientID, handshakeErr = h.ServerSideHandshake(framingAdapter)
		if handshakeErr == nil {
			break
		}
		if errors.Is(handshakeErr, noise.ErrCookieRequired) && attempt == 0 {
			r.logger.Printf("TCP: %s cookie sent, awaiting retry", conn.RemoteAddr())
			continue
		}
		_ = framingAdapter.Close()
		return nil, nil, fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), handshakeErr)
	}

	internalIP, allocErr := ip.AllocateClientIP(r.interfaceSubnet, clientID)
	if allocErr != nil {
		_ = framingAdapter.Close()
		return nil, nil, fmt.Errorf("client %s IP allocation failed: %w", conn.RemoteAddr(), allocErr)
	}
	r.logger.Printf("TCP: %s registered as %s", conn.RemoteAddr(), internalIP)

	cryptographyService, rekeyCtrl, cryptographyServiceErr := r.cryptographyFactory.FromHandshake(h, true)
	if cryptographyServiceErr != nil {
		_ = framingAdapter.Close()
		return nil, nil, fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), cryptographyServiceErr)
	}

	// If session not found, or client is using a new (IP, port) address (e.g., after NAT rebinding), re-register the client.
	existingPeer, getErr := r.sessionManager.GetByInternalAddrPort(internalIP)
	if getErr == nil {
		r.sessionManager.Delete(existingPeer)
		r.logger.Printf("Replacing existing session for %s", internalIP)
	} else if !errors.Is(getErr, session.ErrNotFound) {
		_ = framingAdapter.Close()
		return nil, nil, fmt.Errorf(
			"connection closed: %s (internal IP %s lookup failed: %v)",
			conn.RemoteAddr(),
			internalIP,
			getErr,
		)
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

	// Add IPv6 address to allowedIPs for dual-stack support
	if r.ipv6Subnet.IsValid() {
		ipv6Addr, ipv6Err := ip.AllocateClientIP(r.ipv6Subnet, clientID)
		if ipv6Err == nil {
			allowedIPs = append(allowedIPs, netip.PrefixFrom(ipv6Addr, 128))
		}
	}

	sess := session.NewSessionWithAuth(cryptographyService, rekeyCtrl, internalIP, tcpAddr.AddrPort(), clientPubKey, allowedIPs)
	egress := connection.NewDefaultEgress(framingAdapter, cryptographyService)
	peer := session.NewPeer(sess, egress)
	r.sessionManager.Add(peer)

	return peer, framingAdapter, nil
}
