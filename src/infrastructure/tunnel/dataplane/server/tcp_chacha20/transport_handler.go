package tcp_chacha20

import (
	"context"
	"io"
	"tungo/application/listeners"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"
	"tungo/infrastructure/tunnel/sessionplane/server/tcp_registration"
)

type TransportHandler struct {
	ctx       context.Context
	settings  settings.Settings
	writer    io.ReadWriteCloser
	listener  listeners.TcpListener
	peerStore session.PeerStore
	logger    logging.Logger
	registrar *tcp_registration.Registrar
}

func NewTransportHandler(
	ctx context.Context,
	settings settings.Settings,
	writer io.ReadWriteCloser,
	listener listeners.TcpListener,
	peerStore session.PeerStore,
	logger logging.Logger,
	registrar *tcp_registration.Registrar,
) transport.Handler {
	return &TransportHandler{
		ctx:       ctx,
		settings:  settings,
		writer:    writer,
		listener:  listener,
		peerStore: peerStore,
		logger:    logger,
		registrar: registrar,
	}
}

// HandleTransport is the TCP dataplane ingress:
// - accepts connections
// - delegates session establishment to the session-plane registrar
// - after establishment, reads ciphertext from the session transport, decrypts, dispatches control-plane, writes to TUN
func (t *TransportHandler) HandleTransport() error {
	defer func() {
		_ = t.listener.Close()
	}()
	t.logger.Info("server listening", "protocol", "TCP", "port", t.settings.Port)

	//using this goroutine to 'unblock' TcpListener.Accept blocking-call
	go func() {
		<-t.ctx.Done() //blocks till ctx.Done signal comes in
		_ = t.listener.Close()
		return
	}()

	for {
		select {
		case <-t.ctx.Done():
			return t.ctx.Err()
		default:
			conn, listenErr := t.listener.Accept()
			if t.ctx.Err() != nil {
				return nil
			}
			if listenErr != nil {
				t.logger.Warn("failed to accept connection", "err", listenErr)
				continue
			}
			go func() {
				peer, tr, err := t.registrar.RegisterClient(conn)
				if err != nil {
					t.logger.Warn("failed to register client", "err", err)
					return
				}
				t.handleClient(t.ctx, peer, tr, t.writer)
			}()
		}
	}
}

func (t *TransportHandler) handleClient(ctx context.Context, peer *session.Peer, tr connection.Transport, tunFile io.ReadWriteCloser) {
	newTCPDataplaneWorker(ctx, peer, tr, tunFile, t.peerStore, t.logger).Run()
}
