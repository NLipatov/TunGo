package tcp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"tungo/application/listeners"
	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/network/tcp/adapters"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

type TransportHandler struct {
	ctx                 context.Context
	settings            settings.Settings
	writer              io.ReadWriteCloser
	listener            listeners.TcpListener
	sessionManager      repository.SessionRepository[connection.Session]
	logger              logging.Logger
	handshakeFactory    connection.HandshakeFactory
	cryptographyFactory connection.CryptoFactory
}

func NewTransportHandler(
	ctx context.Context,
	settings settings.Settings,
	writer io.ReadWriteCloser,
	listener listeners.TcpListener,
	sessionManager repository.SessionRepository[connection.Session],
	logger logging.Logger,
	handshakeFactory connection.HandshakeFactory,
	cryptographyFactory connection.CryptoFactory,
) transport.Handler {
	return &TransportHandler{
		ctx:                 ctx,
		settings:            settings,
		writer:              writer,
		listener:            listener,
		sessionManager:      sessionManager,
		logger:              logger,
		handshakeFactory:    handshakeFactory,
		cryptographyFactory: cryptographyFactory,
	}
}

func (t *TransportHandler) HandleTransport() error {
	defer func() {
		_ = t.listener.Close()
	}()
	t.logger.Printf("server listening on port %s (TCP)", t.settings.Port)

	//using this goroutine to 'unblock' TcpListener.Accept blocking-call
	go func() {
		<-t.ctx.Done() //blocks till ctx.Done signal comes in
		_ = t.listener.Close()
		return
	}()

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			conn, listenErr := t.listener.Accept()
			if t.ctx.Err() != nil {
				t.logger.Printf("exiting Accept loop: %s", t.ctx.Err())
				return nil
			}
			if listenErr != nil {
				t.logger.Printf("failed to accept connection: %v", listenErr)
				continue
			}
			go func() {
				err := t.registerClient(conn, t.writer, t.ctx)
				if err != nil {
					t.logger.Printf("failed to register client: %v", err)
				}
			}()
		}
	}
}

func (t *TransportHandler) registerClient(conn net.Conn, tunFile io.ReadWriteCloser, ctx context.Context) error {
	t.logger.Printf("connected: %s", conn.RemoteAddr())

	framingAdapter, fErr := adapters.NewLengthPrefixFramingAdapter(conn, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
	if fErr != nil {
		return fErr
	}
	h := t.handshakeFactory.NewHandshake()
	internalIP, handshakeErr := h.ServerSideHandshake(framingAdapter)
	if handshakeErr != nil {
		_ = conn.Close()
		return fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), handshakeErr)
	}
	t.logger.Printf("registered: %s", conn.RemoteAddr())

	cryptographyService, cryptographyServiceErr := t.cryptographyFactory.FromHandshake(h, true)
	if cryptographyServiceErr != nil {
		_ = conn.Close()
		return fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), cryptographyServiceErr)
	}

	addr := conn.RemoteAddr()
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		_ = conn.Close()
		return fmt.Errorf("invalid remote address type: %T", addr)
	}

	intIP, intIPOk := netip.AddrFromSlice(internalIP)
	if !intIPOk {
		_ = conn.Close()
		return fmt.Errorf("invalid internal IP from handshake")
	}

	// If session not found, or client is using a new (IP, port) address (e.g., after NAT rebinding), re-register the client.
	existingSession, getErr := t.sessionManager.GetByInternalAddrPort(intIP)
	if getErr == nil {
		_ = conn.Close()
		t.sessionManager.Delete(existingSession)
		t.logger.Printf("Replacing existing session for %s", intIP)
	} else if !errors.Is(getErr, repository.ErrSessionNotFound) {
		_ = conn.Close()
		return fmt.Errorf(
			"connection closed: %s (internal IP %s lookup failed: %v)",
			conn.RemoteAddr(),
			internalIP,
			getErr,
		)
	}

	storedSession := NewSession(framingAdapter, cryptographyService, intIP, tcpAddr.AddrPort())

	t.sessionManager.Add(storedSession)

	t.handleClient(ctx, storedSession, tunFile)

	return nil
}

func (t *TransportHandler) handleClient(ctx context.Context, session connection.Session, tunFile io.ReadWriteCloser) {
	defer func() {
		t.sessionManager.Delete(session)
		_ = session.Transport().Close()
		t.logger.Printf("disconnected: %s", session.ExternalAddrPort())
	}()

	buffer := make([]byte, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := session.Transport().Read(buffer)
			if err != nil {
				if err != io.EOF {
					t.logger.Printf("failed to read from client: %v", err)
				}
				return
			}
			if n < chacha20poly1305.Overhead || n > settings.DefaultEthernetMTU+settings.TCPChacha20Overhead {
				t.logger.Printf("invalid ciphertext length: %d", n)
				continue
			}
			pt, err := session.Crypto().Decrypt(buffer[:n])
			if err != nil {
				t.logger.Printf("failed to decrypt data: %s", err)
				continue
			}
			if _, err = tunFile.Write(pt); err != nil {
				t.logger.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
