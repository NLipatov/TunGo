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
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/tcp/adapters"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
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
	servicePacket       service.PacketHandler
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
		servicePacket:       service.NewDefaultPacketHandler(),
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

func (t *TransportHandler) registerClient(conn net.Conn, tunFile io.ReadWriteCloser, ctx context.Context) error {
	t.logger.Printf("TCP: %s connected", conn.RemoteAddr())

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
	t.logger.Printf("TCP: %s registered as %s", conn.RemoteAddr(), internalIP)

	cryptographyService, rekeyCtrl, cryptographyServiceErr := t.cryptographyFactory.FromHandshake(h, true)
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

	storedSession := NewSession(framingAdapter, cryptographyService, rekeyCtrl, intIP, tcpAddr.AddrPort())

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
			if rc := session.RekeyController(); rc != nil {
				if spType, spOk := service.NewDefaultPacketHandler().TryParseType(pt); spOk {
					if spType == service.RekeyInit {
						t.handleRekeyInit(rc, session, pt)
						continue
					}
					// server ignores Ack
				}
			}
			if _, err = tunFile.Write(pt); err != nil {
				t.logger.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}

func (t *TransportHandler) handleRekeyInit(rc *rekey.StateMachine, session connection.Session, pt []byte) {
	if len(pt) < service.RekeyPacketLen {
		t.logger.Printf("rekey init packet too short: %d bytes", len(pt))
		return
	}
	var clientPub [service.RekeyPublicKeyLen]byte
	copy(clientPub[:], pt[3:service.RekeyPacketLen])
	serverPub, serverPriv, err := t.handshakeCrypto.GenerateX25519KeyPair()
	if err != nil {
		t.logger.Printf("rekey init: failed to generate server key pair: %v", err)
		return
	}
	shared, err := curve25519.X25519(serverPriv[:], clientPub[:])
	if err != nil {
		t.logger.Printf("rekey init: failed to derive shared: %v", err)
		return
	}
	if rc.LastRekeyEpoch >= 65000 {
		t.logger.Printf("rekey init: epoch exhausted, sending session reset")
		t.sendSessionReset(session)
		return
	}
	currentC2S := rc.CurrentClientToServerKey()
	currentS2C := rc.CurrentServerToClientKey()
	newC2S, err := t.handshakeCrypto.DeriveKey(shared, currentC2S, []byte("tungo-rekey-c2s"))
	if err != nil {
		t.logger.Printf("rekey init: derive key failed: %v", err)
		return
	}
	newS2C, err := t.handshakeCrypto.DeriveKey(shared, currentS2C, []byte("tungo-rekey-s2c"))
	if err != nil {
		t.logger.Printf("rekey init: derive key failed: %v", err)
		return
	}
	sendKey := newC2S
	recvKey := newS2C
	if rc.IsServer {
		sendKey, recvKey = newS2C, newC2S
	}
	epoch, err := rc.StartRekey(sendKey, recvKey)
	if err != nil {
		t.logger.Printf("rekey init: install/apply failed: %v", err)
		return
	}

	ackPayload := make([]byte, service.RekeyPacketLen)
	copy(ackPayload[3:], serverPub)
	sp, err := service.NewDefaultPacketHandler().EncodeV1(service.RekeyAck, ackPayload)
	if err != nil {
		t.logger.Printf("rekey init: encode ack failed: %v", err)
		return
	}
	enc, err := session.Crypto().Encrypt(sp)
	if err != nil {
		t.logger.Printf("rekey init: encrypt ack failed: %v", err)
		return
	}
	if _, err := session.Transport().Write(enc); err != nil {
		t.logger.Printf("rekey init: write ack failed: %v", err)
	}
	// now it's safe to switch send for TCP
	rc.ActivateSendEpoch(epoch)
}

// sendSessionReset sends a SessionReset service packet to the given session.
func (t *TransportHandler) sendSessionReset(session connection.Session) {
	servicePacketBuffer := make([]byte, 3)
	servicePacketPayload, err := t.servicePacket.EncodeLegacy(service.SessionReset, servicePacketBuffer)
	if err != nil {
		t.logger.Printf("failed to encode legacy session reset service packet: %v", err)
		return
	}
	_, _ = session.Transport().Write(servicePacketPayload)
}
