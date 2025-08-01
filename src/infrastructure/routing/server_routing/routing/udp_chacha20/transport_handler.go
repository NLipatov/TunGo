package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"tungo/application"
	"tungo/application/listeners"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"
)

type TransportHandler struct {
	ctx                 context.Context
	settings            settings.Settings
	writer              io.Writer
	sessionManager      repository.SessionRepository[application.Session]
	logger              application.Logger
	listenerConn        listeners.UdpListener
	handshakeFactory    application.HandshakeFactory
	cryptographyFactory application.CryptographyServiceFactory
}

func NewTransportHandler(
	ctx context.Context,
	settings settings.Settings,
	writer io.Writer,
	listenerConn listeners.UdpListener,
	sessionManager repository.SessionRepository[application.Session],
	logger application.Logger,
	handshakeFactory application.HandshakeFactory,
	cryptographyFactory application.CryptographyServiceFactory,
) application.TransportHandler {
	return &TransportHandler{
		ctx:                 ctx,
		settings:            settings,
		writer:              writer,
		sessionManager:      sessionManager,
		logger:              logger,
		listenerConn:        listenerConn,
		handshakeFactory:    handshakeFactory,
		cryptographyFactory: cryptographyFactory,
	}
}

func (t *TransportHandler) HandleTransport() error {
	defer func(conn listeners.UdpListener) {
		_ = conn.Close()
	}(t.listenerConn)

	t.logger.Printf("server listening on port %s (UDP)", t.settings.Port)

	go func() {
		<-t.ctx.Done()
		_ = t.listenerConn.Close()
	}()

	dataBuf := make([]byte, network.MaxPacketLengthBytes+12)
	oobBuf := make([]byte, 1024)

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, _, _, clientAddr, readFromUdpErr := t.listenerConn.ReadMsgUDPAddrPort(dataBuf, oobBuf)
			if readFromUdpErr != nil {
				if t.ctx.Done() != nil {
					return nil
				}

				t.logger.Printf("failed to read from UDP: %s", readFromUdpErr)
				continue
			}

			if n == 0 {
				t.logger.Printf("packet dropped: empty packet from %v", clientAddr.String())
				continue
			}

			_ = t.handlePacket(t.listenerConn, clientAddr, dataBuf[:n])
		}
	}
}

// handlePacket processes a UDP packet from addrPort.
// Registers the client if needed, or decrypts and forwards the packet for an existing session.
func (t *TransportHandler) handlePacket(
	conn listeners.UdpListener,
	addrPort netip.AddrPort,
	packet []byte) error {
	session, sessionLookupErr := t.sessionManager.GetByExternalAddrPort(addrPort)
	// If session not found, or client is using a new (IP, port) address (e.g., after NAT rebinding), re-register the client.
	if sessionLookupErr != nil ||
		session.ExternalAddrPort() != addrPort {
		// Pass initial data to registration function
		regErr := t.registerClient(conn, addrPort, packet)
		if regErr != nil {
			t.logger.Printf("host %v failed registration: %v", addrPort.Addr().AsSlice(), regErr)
			_, _ = conn.WriteToUDPAddrPort([]byte{
				byte(network.SessionReset),
			}, addrPort)
			return regErr
		}
		return nil
	}

	// Handle client data
	decrypted, decryptionErr := session.CryptographyService().Decrypt(packet)
	if decryptionErr != nil {
		t.logger.Printf("failed to decrypt data: %v", decryptionErr)
		return decryptionErr
	}

	// Write the decrypted packet to the TUN interface
	_, err := t.writer.Write(decrypted)
	if err != nil {
		t.logger.Printf("failed to write to TUN: %v", err)
		return err
	}

	return nil
}

func (t *TransportHandler) registerClient(
	conn listeners.UdpListener,
	addrPort netip.AddrPort,
	initialData []byte) error {
	_ = conn.SetReadBuffer(65536)
	_ = conn.SetWriteBuffer(65536)

	// Pass initialData and addrPort to the crypto function
	h := t.handshakeFactory.NewHandshake()
	adapter := network.NewInitialDataAdapter(
		network.NewUdpAdapter(conn, addrPort), initialData)
	internalIP, handshakeErr := h.ServerSideHandshake(adapter)
	if handshakeErr != nil {
		return handshakeErr
	}

	cryptoSession, cryptoSessionErr := t.cryptographyFactory.FromHandshake(h, true)
	if cryptoSessionErr != nil {
		return cryptoSessionErr
	}

	intIp, intIpOk := netip.AddrFromSlice(internalIP)
	if !intIpOk {
		return fmt.Errorf("failed to parse internal IP: %v", internalIP)
	}

	t.sessionManager.Add(NewSession(adapter, cryptoSession, intIp, addrPort))

	t.logger.Printf("%v registered as: %v", addrPort.Addr(), internalIP)

	return nil
}
