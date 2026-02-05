package tcp_registration

import (
	"errors"
	"fmt"
	"net"
	"net/netip"

	"tungo/application/logging"
	"tungo/application/network/connection"
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
	sessionManager      session.Repository
}

func NewRegistrar(
	logger logging.Logger,
	handshakeFactory connection.HandshakeFactory,
	cryptographyFactory connection.CryptoFactory,
	sessionManager session.Repository,
) *Registrar {
	return &Registrar{
		logger:              logger,
		handshakeFactory:    handshakeFactory,
		cryptographyFactory: cryptographyFactory,
		sessionManager:      sessionManager,
	}
}

// RegisterClient performs the handshake/crypto handshake on conn,
// creates a Peer, adds it to the repository, and returns it alongside
// the framing transport. The caller is responsible for driving the
// dataplane loop using the returned peer and transport.
func (r *Registrar) RegisterClient(conn net.Conn) (*session.Peer, connection.Transport, error) {
	r.logger.Printf("TCP: %s connected", conn.RemoteAddr())

	framingAdapter, fErr := adapters.NewLengthPrefixFramingAdapter(conn, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
	if fErr != nil {
		_ = conn.Close() // Prevent socket leak on framing adapter failure
		return nil, nil, fErr
	}
	h := r.handshakeFactory.NewHandshake()
	internalIP, handshakeErr := h.ServerSideHandshake(framingAdapter)
	if handshakeErr != nil {
		_ = framingAdapter.Close()
		return nil, nil, fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), handshakeErr)
	}
	r.logger.Printf("TCP: %s registered as %s", conn.RemoteAddr(), internalIP)

	cryptographyService, rekeyCtrl, cryptographyServiceErr := r.cryptographyFactory.FromHandshake(h, true)
	if cryptographyServiceErr != nil {
		_ = framingAdapter.Close()
		return nil, nil, fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), cryptographyServiceErr)
	}

	addr := conn.RemoteAddr()
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		_ = framingAdapter.Close()
		return nil, nil, fmt.Errorf("invalid remote address type: %T", addr)
	}

	// If session not found, or client is using a new (IP, port) address (e.g., after NAT rebinding), re-register the client.
	existingPeer, getErr := r.sessionManager.GetByInternalAddrPort(internalIP)
	if getErr == nil {
		_ = existingPeer.Egress().Close()
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

	sess := session.NewSessionWithAuth(cryptographyService, rekeyCtrl, internalIP, tcpAddr.AddrPort(), clientPubKey, allowedIPs)
	egress := connection.NewDefaultEgress(framingAdapter, cryptographyService)
	peer := session.NewPeer(sess, egress)
	r.sessionManager.Add(peer)

	return peer, framingAdapter, nil
}
