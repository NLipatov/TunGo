package tcp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"tungo/application"
	"tungo/infrastructure/network"
	"tungo/infrastructure/network/framing"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"
)

type TransportHandler struct {
	ctx                 context.Context
	settings            settings.Settings
	writer              io.ReadWriteCloser
	listener            application.TcpListener
	sessionManager      repository.SessionRepository[Session]
	Logger              application.Logger
	handshakeFactory    application.HandshakeFactory
	cryptographyFactory application.CryptographyServiceFactory
}

func NewTransportHandler(
	ctx context.Context,
	settings settings.Settings,
	writer io.ReadWriteCloser,
	listener application.TcpListener,
	sessionManager repository.SessionRepository[Session],
	logger application.Logger,
	handshakeFactory application.HandshakeFactory,
	cryptographyFactory application.CryptographyServiceFactory,
) application.TransportHandler {
	return &TransportHandler{
		ctx:                 ctx,
		settings:            settings,
		writer:              writer,
		listener:            listener,
		sessionManager:      sessionManager,
		Logger:              logger,
		handshakeFactory:    handshakeFactory,
		cryptographyFactory: cryptographyFactory,
	}
}

func (t *TransportHandler) HandleTransport() error {
	defer func() {
		_ = t.listener.Close()
	}()
	t.Logger.Printf("server listening on port %s (TCP)", t.settings.Port)

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
				t.Logger.Printf("exiting Accept loop: %s", t.ctx.Err())
				return nil
			}
			if listenErr != nil {
				t.Logger.Printf("failed to accept connection: %v", listenErr)
				continue
			}
			go func() {
				err := t.registerClient(conn, t.writer, t.ctx)
				if err != nil {
					t.Logger.Printf("failed to register client: %v", err)
				}
			}()
		}
	}
}

func (t *TransportHandler) registerClient(conn net.Conn, tunFile io.ReadWriteCloser, ctx context.Context) error {
	t.Logger.Printf("connected: %s", conn.RemoteAddr())

	h := t.handshakeFactory.NewHandshake()
	internalIP, handshakeErr := h.ServerSideHandshake(&network.TcpAdapter{
		Conn: conn,
	})
	if handshakeErr != nil {
		_ = conn.Close()
		return fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), handshakeErr)
	}
	t.Logger.Printf("registered: %s", conn.RemoteAddr())

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
		t.Logger.Printf("Replacing existing session for %s", intIP)
	} else if !errors.Is(getErr, repository.ErrSessionNotFound) {
		_ = conn.Close()
		return fmt.Errorf(
			"connection closed: %s (internal IP %s lookup failed: %v)",
			conn.RemoteAddr(),
			internalIP,
			getErr,
		)
	}

	storedSession := Session{
		conn:                conn,
		CryptographyService: cryptographyService,
		internalIP:          intIP,
		externalIP:          tcpAddr.AddrPort(),
	}

	t.sessionManager.Add(storedSession)

	t.handleClient(ctx, storedSession, tunFile)

	return nil
}

func (t *TransportHandler) handleClient(ctx context.Context, session Session, tunFile io.ReadWriteCloser) {
	defer func() {
		t.sessionManager.Delete(session)
		_ = session.conn.Close()
		t.Logger.Printf("disconnected: %s", session.conn.RemoteAddr())
	}()

	buf := make([]byte, network.MaxPacketLengthBytes)
	adapter := framing.NewTCPFramingAdapter(session.conn)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := adapter.Read(buf)
			if err != nil {
				if err != io.EOF {
					t.Logger.Printf("failed to read from client: %v", err)
				}
				return
			}

			decrypted, decryptionErr := session.CryptographyService.Decrypt(buf[:n])
			if decryptionErr != nil {
				t.Logger.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			_, err = tunFile.Write(decrypted)
			if err != nil {
				t.Logger.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
