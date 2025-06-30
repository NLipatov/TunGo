package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"tungo/application"
	"tungo/infrastructure/PAL/server_configuration"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/listeners/udp_listener"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

type TransportHandler struct {
	ctx            context.Context
	settings       settings.Settings
	writer         io.Writer
	sessionManager session_management.WorkerSessionManager[Session]
	logger         application.Logger
	listener       udp_listener.Listener
}

func NewTransportHandler(
	ctx context.Context,
	settings settings.Settings,
	writer io.Writer,
	listener udp_listener.Listener,
	sessionManager session_management.WorkerSessionManager[Session],
	logger application.Logger,
) application.TransportHandler {
	return &TransportHandler{
		ctx:            ctx,
		settings:       settings,
		writer:         writer,
		sessionManager: sessionManager,
		logger:         logger,
		listener:       listener,
	}
}

func (t *TransportHandler) HandleTransport() error {
	conn, err := t.listener.ListenUDP()
	if err != nil {
		t.logger.Printf("failed to listen on port: %s", err)
	}
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)

	t.logger.Printf("server listening on port %s (UDP)", t.settings.Port)

	go func() {
		<-t.ctx.Done()
		_ = conn.Close()
	}()

	dataBuf := make([]byte, network.MaxPacketLengthBytes+12)
	oobBuf := make([]byte, 1024)

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, _, _, clientAddr, readFromUdpErr := conn.ReadMsgUDPAddrPort(dataBuf, oobBuf)
			if readFromUdpErr != nil {
				if t.ctx.Done() != nil {
					return nil
				}

				t.logger.Printf("failed to read from UDP: %s", readFromUdpErr)
				continue
			}

			clientSession, getErr := t.sessionManager.GetByExternalIP(clientAddr.Addr())
			if getErr != nil || clientSession.remoteAddrPort.Port() != clientAddr.Port() {
				// Pass initial data to registration function
				regErr := t.registerClient(conn, clientAddr, dataBuf[:n])
				if regErr != nil {
					t.logger.Printf("host %v failed registration: %v", clientAddr.Addr().AsSlice(), regErr)
					_, _ = conn.WriteToUDPAddrPort([]byte{
						byte(network.SessionReset),
					}, clientAddr)
				}
				continue
			}

			// Handle client data
			decrypted, decryptionErr := clientSession.CryptographyService.Decrypt(dataBuf[:n])
			if decryptionErr != nil {
				t.logger.Printf("failed to decrypt data: %v", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = t.writer.Write(decrypted)
			if err != nil {
				t.logger.Printf("failed to write to TUN: %v", err)
			}
		}
	}
}

func (t *TransportHandler) registerClient(conn *net.UDPConn, clientAddr netip.AddrPort, initialData []byte) error {
	_ = conn.SetReadBuffer(65536)
	_ = conn.SetWriteBuffer(65536)

	// Pass initialData and clientAddr to the crypto function
	h := handshake.NewHandshake()
	adapter := network.NewInitialDataAdapter(
		network.NewUdpAdapter(conn, clientAddr), initialData)
	internalIP, handshakeErr := h.ServerSideHandshake(adapter)
	if handshakeErr != nil {
		return handshakeErr
	}

	serverConfigurationManager := server_configuration.NewManager(server_configuration.NewServerResolver())
	_, err := serverConfigurationManager.Configuration()
	if err != nil {
		return fmt.Errorf("failed to read server configuration: %s", err)
	}

	cryptoSession, cryptoSessionErr := chacha20.NewUdpSession(h.Id(), h.ServerKey(), h.ClientKey(), true)
	if cryptoSessionErr != nil {
		return cryptoSessionErr
	}

	intIp, intIpOk := netip.AddrFromSlice(internalIP)
	if !intIpOk {
		return fmt.Errorf("failed to parse internal IP: %v", internalIP)
	}

	t.sessionManager.Add(Session{
		connectionAdapter: &network.ServerUdpAdapter{
			UdpConn:  conn,
			AddrPort: clientAddr,
		},
		remoteAddrPort:      clientAddr,
		CryptographyService: cryptoSession,
		internalIP:          intIp,
		externalIP:          clientAddr.Addr(),
	})

	t.logger.Printf("%v registered as: %v", clientAddr.Addr(), internalIP)

	return nil
}

func (t *TransportHandler) extractIPv4(ip net.IP) ([4]byte, error) {
	if len(ip) == 16 && t.isIPv4Mapped(ip) {
		return [4]byte(ip[12:16]), nil
	}
	if len(ip) == 4 {
		return [4]byte(ip), nil
	}
	return [4]byte(make([]byte, 4)), nil
}

func (t *TransportHandler) isIPv4Mapped(ip net.IP) bool {
	// check for ::ffff:0:0/96 prefix
	return ip[0] == 0 && ip[1] == 0 && ip[2] == 0 &&
		ip[3] == 0 && ip[4] == 0 && ip[5] == 0 &&
		ip[6] == 0 && ip[7] == 0 && ip[8] == 0 &&
		ip[9] == 0 && ip[10] == 0xff && ip[11] == 0xff
}
