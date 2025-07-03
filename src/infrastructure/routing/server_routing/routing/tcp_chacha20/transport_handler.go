package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"tungo/application"
	"tungo/infrastructure/PAL/server_configuration"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/listeners/tcp_listener"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

type TransportHandler struct {
	ctx                  context.Context
	settings             settings.Settings
	writer               io.ReadWriteCloser
	listener             tcp_listener.Listener
	sessionManager       session_management.WorkerSessionManager[Session]
	Logger               application.Logger
	configuration        *server_configuration.Configuration
	configurationManager server_configuration.ServerConfigurationManager
}

func NewTransportHandler(
	ctx context.Context,
	settings settings.Settings,
	writer io.ReadWriteCloser,
	listener tcp_listener.Listener,
	sessionManager session_management.WorkerSessionManager[Session],
	logger application.Logger,
	manager server_configuration.ServerConfigurationManager,
) application.TransportHandler {
	return &TransportHandler{
		ctx:                  ctx,
		settings:             settings,
		writer:               writer,
		listener:             listener,
		sessionManager:       sessionManager,
		Logger:               logger,
		configurationManager: manager,
	}
}

func (t *TransportHandler) HandleTransport() error {
	defer func() {
		_ = t.listener.Close()
	}()
	t.Logger.Printf("server listening on port %s (TCP)", t.settings.Port)

	//using this goroutine to 'unblock' Listener.Accept blocking-call
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

	conf, confErr := t.configurationManager.Configuration()
	if confErr != nil {
		return confErr
	}
	t.configuration = conf

	h := handshake.NewHandshake(t.configuration.Ed25519PublicKey, t.configuration.Ed25519PrivateKey)
	internalIP, handshakeErr := h.ServerSideHandshake(&network.TcpAdapter{
		Conn: conn,
	})
	if handshakeErr != nil {
		_ = conn.Close()
		return fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), handshakeErr)
	}
	t.Logger.Printf("registered: %s", conn.RemoteAddr())

	cryptographyService, cryptographyServiceErr := chacha20.NewTcpCryptographyService(
		h.Id(),
		h.ServerKey(),
		h.ClientKey(),
		true,
	)
	if cryptographyServiceErr != nil {
		_ = conn.Close()
		return fmt.Errorf("client %s failed registration: %w", conn.RemoteAddr(), cryptographyServiceErr)
	}

	tcpConn := conn.(*net.TCPConn)
	addr := tcpConn.RemoteAddr().(*net.TCPAddr)
	intIP, intIPOk := netip.AddrFromSlice(internalIP)
	if !intIPOk {
		_ = tcpConn.Close()
		return fmt.Errorf("invalid internal IP from handshake")
	}

	// Prevent IP spoofing
	_, getErr := t.sessionManager.GetByInternalIP(intIP)
	if !errors.Is(getErr, session_management.ErrSessionNotFound) {
		_ = conn.Close()
		return fmt.Errorf(
			"connection closed: %s (internal internalIP %s already in use)\n",
			conn.RemoteAddr(),
			internalIP,
		)
	}

	storedSession := Session{
		conn:                conn,
		CryptographyService: cryptographyService,
		internalIP:          intIP,
		externalIP:          addr.AddrPort(),
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
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Read the length of the encrypted packet (4 bytes)
			_, err := io.ReadFull(session.conn, buf[:4])
			if err != nil {
				if err != io.EOF {
					t.Logger.Printf("failed to read from client: %v", err)
				}
				return
			}

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buf[:4])
			if length < 4 || length > network.MaxPacketLengthBytes {
				t.Logger.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(session.conn, buf[:length])
			if err != nil {
				t.Logger.Printf("failed to read packet from connection: %s", err)
				continue
			}

			decrypted, decryptionErr := session.CryptographyService.Decrypt(buf[:length])
			if decryptionErr != nil {
				t.Logger.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = tunFile.Write(decrypted)
			if err != nil {
				t.Logger.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
