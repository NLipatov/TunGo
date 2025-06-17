package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/listeners/tcp_listener"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

type TransportHandler struct {
	ctx            context.Context
	settings       settings.Settings
	writer         io.ReadWriteCloser
	listener       tcp_listener.Listener
	sessionManager session_management.WorkerSessionManager[Session]
	Logger         application.Logger
}

func NewTransportHandler(
	ctx context.Context,
	settings settings.Settings,
	writer io.ReadWriteCloser,
	listener tcp_listener.Listener,
	sessionManager session_management.WorkerSessionManager[Session],
	logger application.Logger,
) application.TransportHandler {
	return &TransportHandler{
		ctx:            ctx,
		settings:       settings,
		writer:         writer,
		listener:       listener,
		sessionManager: sessionManager,
		Logger:         logger,
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
			go t.registerClient(conn, t.writer, t.ctx)
		}
	}
}
func (t *TransportHandler) registerClient(conn net.Conn, tunFile io.ReadWriteCloser, ctx context.Context) {
	t.Logger.Printf("connected: %s", conn.RemoteAddr())
	h := handshake.NewHandshake()
	internalIP, handshakeErr := h.ServerSideHandshake(&network.TcpAdapter{
		Conn: conn,
	})
	if handshakeErr != nil {
		_ = conn.Close()
		t.Logger.Printf("connection closed: %s (regfail: %s)\n", conn.RemoteAddr(), handshakeErr)
		return
	}
	t.Logger.Printf("registered: %s", conn.RemoteAddr())

	cryptographyService, cryptographyServiceErr := chacha20.NewTcpCryptographyService(h.Id(), h.ServerKey(), h.ClientKey(), true)
	if cryptographyServiceErr != nil {
		_ = conn.Close()
		t.Logger.Printf("connection closed: %s (regfail: %s)\n", conn.RemoteAddr(), cryptographyServiceErr)
	}

	tcpConn := conn.(*net.TCPConn)
	addr := tcpConn.RemoteAddr().(*net.TCPAddr)
	internalIP = internalIP.To4()
	externalIP := addr.IP.To4()

	// Prevent IP spoofing
	_, getErr := t.sessionManager.GetByInternalIP(internalIP)
	if !errors.Is(getErr, session_management.ErrSessionNotFound) {
		t.Logger.Printf("connection closed: %s (internal internalIP %s already in use)\n", conn.RemoteAddr(), internalIP)
		_ = conn.Close()
	}

	storedSession := Session{
		conn:                conn,
		CryptographyService: cryptographyService,
		internalIP:          internalIP.To4(),
		externalIP:          externalIP.To4(),
	}

	t.sessionManager.Add(storedSession)

	t.handleClient(ctx, storedSession, tunFile)
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

			// Retrieve the session for this client
			sessionValue, getErr := t.sessionManager.GetByInternalIP(session.InternalIP())
			if getErr != nil {
				t.Logger.Printf("failed to load session for IP %s", sessionValue.InternalIP())
				continue
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

			decrypted, decryptionErr := sessionValue.CryptographyService.Decrypt(buf[:length])
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
