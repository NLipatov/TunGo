package tcp

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"

	"tungo/application/configuration/settings"
	"tungo/application/network/ip"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/network/tcp/adapters"
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

type crypto interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

type rekeyController interface {
	ReadyForRekey() bool
	SendEpoch() uint16
	StartRekey(c2s, s2c []byte) (uint16, error)
	ActivateSendEpoch(uint16)
	ObservePeerEpoch(uint16)
	CurrentKeys() (clientToServer, serverToClient []byte)
}

type newHandshake func() handshake
type newCrypto func(chacha20.KeyMaterial, bool) (crypto, rekeyController, error)

// registrar turns an untrusted net.Conn into an established Peer
// (handshake + crypto init + session repo add).
type registrar struct {
	newHandshake    newHandshake
	newCrypto       newCrypto
	sessionManager  tcpRegistrationRepo
	interfaceSubnet netip.Prefix
	ipv6Subnet      netip.Prefix
}

type tcpRegistrationRepo interface {
	Add(*session.Peer)
	Delete(*session.Peer)
	GetByInternalAddrPort(netip.Addr) (*session.Peer, error)
}

func newRegistrar(
	newHandshake newHandshake,
	newCrypto newCrypto,
	sessionManager tcpRegistrationRepo,
	interfaceSubnet netip.Prefix,
	ipv6Subnet netip.Prefix,
) *registrar {
	return &registrar{
		newHandshake:    newHandshake,
		newCrypto:       newCrypto,
		sessionManager:  sessionManager,
		interfaceSubnet: interfaceSubnet,
		ipv6Subnet:      ipv6Subnet,
	}
}

// RegisterClient performs the handshake/crypto handshake on conn,
// creates a Peer, adds it to the repository, and returns it alongside
// the framing transport. The caller is responsible for driving the
// packet loop using the returned peer and transport.
func (r *registrar) registerClient(conn net.Conn) (*session.Peer, io.ReadWriteCloser, error) {
	slog.Info("TCP client connected", "remote_addr", conn.RemoteAddr())

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
	var h handshake
	var clientID int
	for attempt := 0; ; attempt++ {
		h = r.newHandshake()
		var handshakeErr error
		clientID, handshakeErr = h.ServerSideHandshake(framingAdapter)
		if handshakeErr == nil {
			break
		}
		if errors.Is(handshakeErr, noise.ErrCookieRequired) && attempt == 0 {
			slog.Warn("TCP cookie sent, awaiting retry", "remote_addr", conn.RemoteAddr())
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
	slog.Info("TCP client registered", "remote_addr", conn.RemoteAddr(), "internal_ip", internalIP)

	cryptographyService, epochController, cryptographyServiceErr := r.newCrypto(h, true)
	if cryptographyServiceErr != nil {
		_ = framingAdapter.Close()
		return nil, nil, fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), cryptographyServiceErr)
	}
	var rekeyCoordinator *rekey.ServerRekeyCoordinator
	if epochController != nil {
		rekeyCoordinator = rekey.NewServerRekeyCoordinator(epochController)
	}

	// If session not found, or client is using a new (IP, port) address (e.g., after NAT rebinding), re-register the client.
	existingPeer, getErr := r.sessionManager.GetByInternalAddrPort(internalIP)
	if getErr == nil {
		r.sessionManager.Delete(existingPeer)
		slog.Info("replacing existing session", "internal_ip", internalIP)
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

	peer := session.NewPeerWithAuth(
		cryptographyService, rekeyCoordinator, internalIP, tcpAddr.AddrPort(), clientPubKey, allowedIPs, framingAdapter,
	)
	r.sessionManager.Add(peer)

	return peer, framingAdapter, nil
}
