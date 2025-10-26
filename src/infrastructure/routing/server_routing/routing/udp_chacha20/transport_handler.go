package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"tungo/application/listeners"
	"tungo/application/logging"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/domain/network/service"
	"tungo/infrastructure/network/udp/adapters"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"
)

type TransportHandler struct {
	ctx                 context.Context
	settings            settings.Settings
	writer              io.Writer
	sessionManager      repository.SessionRepository[connection.Session]
	logger              logging.Logger
	listenerConn        listeners.UdpListener
	handshakeFactory    connection.HandshakeFactory
	cryptographyFactory connection.CryptoFactory
	servicePacket       service.PacketHandler
	spBuffer            [3]byte
	packetBuffer        []byte
	oobBuffer           []byte
	mtu                 int
}

func NewTransportHandler(
	ctx context.Context,
	workerSettings settings.Settings,
	writer io.Writer,
	listenerConn listeners.UdpListener,
	sessionManager repository.SessionRepository[connection.Session],
	logger logging.Logger,
	handshakeFactory connection.HandshakeFactory,
	cryptographyFactory connection.CryptoFactory,
	servicePacket service.PacketHandler,
) transport.Handler {
	resolvedMTU := settings.ResolveMTU(workerSettings.MTU)
	return &TransportHandler{
		ctx:                 ctx,
		settings:            workerSettings,
		writer:              writer,
		sessionManager:      sessionManager,
		logger:              logger,
		listenerConn:        listenerConn,
		handshakeFactory:    handshakeFactory,
		cryptographyFactory: cryptographyFactory,
		servicePacket:       servicePacket,
		packetBuffer:        make([]byte, settings.UDPBufferSize(resolvedMTU)),
		oobBuffer:           make([]byte, 1024),
		mtu:                 resolvedMTU,
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

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, _, _, clientAddr, readFromUdpErr := t.listenerConn.ReadMsgUDPAddrPort(t.packetBuffer[:], t.oobBuffer[:])
			if readFromUdpErr != nil {
				if t.ctx.Err() != nil {
					return t.ctx.Err()
				}
				t.logger.Printf("failed to read from UDP: %s", readFromUdpErr)
				continue
			}
			if n == 0 {
				t.logger.Printf("packet dropped: empty packet from %v", clientAddr.String())
				continue
			}
			_ = t.handlePacket(t.listenerConn, clientAddr, t.packetBuffer[:n])
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
			servicePacketPayload, servicePacketErr := t.servicePacket.EncodeLegacy(service.SessionReset, t.spBuffer[:])
			if servicePacketErr != nil {
				t.logger.Printf("failed to encode legacy session reset service packet: %v", servicePacketErr)
				return regErr
			}
			_, _ = conn.WriteToUDPAddrPort(servicePacketPayload, addrPort)
			return regErr
		}
		return nil
	}

	// Handle client data
	decrypted, decryptionErr := session.Crypto().Decrypt(packet)
	if decryptionErr != nil {
		t.logger.Printf("failed to decrypt data: %v", decryptionErr)
		t.sessionManager.Delete(session)
		servicePacketPayload, servicePacketErr := t.servicePacket.EncodeLegacy(service.SessionReset, t.spBuffer[:])
		if servicePacketErr != nil {
			t.logger.Printf("failed to encode legacy session reset service packet: %v", servicePacketErr)
			return decryptionErr
		}
		if _, writeErr := conn.WriteToUDPAddrPort(servicePacketPayload, addrPort); writeErr != nil {
			t.logger.Printf("failed to send session reset to %v: %v", addrPort, writeErr)
		}
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
	adapter := adapters.NewInitialDataAdapter(
		adapters.NewUdpAdapter(conn, addrPort, t.mtu), initialData)
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

	negotiatedMTU := t.mtu
	if peerMTU, ok := h.PeerMTU(); ok && peerMTU > 0 && peerMTU < negotiatedMTU {
		negotiatedMTU = peerMTU
	}

	t.sessionManager.Add(NewSession(adapter, cryptoSession, intIp, addrPort, negotiatedMTU))

	t.logger.Printf("%v registered as: %v", addrPort.Addr(), internalIP)

	return nil
}
