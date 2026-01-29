package tcp_chacha20

import (
	"context"
	"io"
	"tungo/application/listeners"
	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/routing/controlplane"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"
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
	handshakeCrypto     handshake.Crypto
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
		handshakeCrypto:     &handshake.DefaultCrypto{},
	}
}

// HandleTransport is the TCP dataplane ingress:
// - accepts connections
// - delegates session establishment to the session-plane (see sessionplane_registration.go)
// - after establishment, reads ciphertext from the session transport, decrypts, dispatches control-plane, writes to TUN
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
			return t.ctx.Err()
		default:
			conn, listenErr := t.listener.Accept()
			if t.ctx.Err() != nil {
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

func (t *TransportHandler) handleClient(ctx context.Context, session connection.Session, tunFile io.ReadWriteCloser) {
	(&tcpDataplaneWorker{
		ctx:            ctx,
		session:        session,
		tunFile:        tunFile,
		sessionManager: t.sessionManager,
		logger:         t.logger,
		onRekeyInit:    t.handleRekeyInit,
	}).Run()
}

func (t *TransportHandler) handleRekeyInit(fsm rekey.FSM, session connection.Session, pt []byte) {
	serverPub, epoch, ok, err := controlplane.ServerHandleRekeyInit(t.handshakeCrypto, fsm, pt)
	if err != nil {
		t.logger.Printf("rekey init: install/apply failed: %v", err)
		return
	}
	if !ok {
		return
	}

	ackPayload := make([]byte, service_packet.RekeyPacketLen)
	copy(ackPayload[3:], serverPub)
	sp, err := service_packet.EncodeV1Header(service_packet.RekeyAck, ackPayload)
	if err != nil {
		t.logger.Printf("rekey init: encode ack failed: %v", err)
		return
	}
	if err := session.Outbound().SendControl(sp); err != nil {
		t.logger.Printf("rekey init: send ack failed: %v", err)
	} else {
		// now it's safe to switch send for TCP
		fsm.ActivateSendEpoch(epoch)
	}
}

// sendSessionReset sends a SessionReset service_packet packet to the given session.
func (t *TransportHandler) sendSessionReset(session connection.Session) {
	servicePacketBuffer := make([]byte, 3)
	servicePacketPayload, err := service_packet.EncodeLegacyHeader(service_packet.SessionReset, servicePacketBuffer)
	if err != nil {
		t.logger.Printf("failed to encode legacy session reset service_packet packet: %v", err)
		return
	}
	_, _ = session.Transport().Write(servicePacketPayload)
}
