package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"tungo/application"
	server_configuration2 "tungo/infrastructure/PAL/server_configuration"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

type UdpTunWorker struct {
	ctx            context.Context
	tun            io.ReadWriteCloser
	settings       settings.Settings
	sessionManager session_management.WorkerSessionManager[session]
}

func NewUdpTunWorker(
	ctx context.Context, tun io.ReadWriteCloser, settings settings.Settings,
) application.TunWorker {
	return &UdpTunWorker{
		tun:            tun,
		ctx:            ctx,
		settings:       settings,
		sessionManager: session_management.NewDefaultWorkerSessionManager[session](),
	}
}

func (u *UdpTunWorker) HandleTun() error {
	packetBuffer := make([]byte, network.MaxPacketLengthBytes+12)
	udpReader := chacha20.NewUdpReader(u.tun)

	destinationAddressBytes := [4]byte{}

	for {
		select {
		case <-u.ctx.Done():
			return nil
		default:
			n, err := udpReader.Read(packetBuffer)
			if err != nil {
				if u.ctx.Done() != nil {
					return nil
				}

				if err == io.EOF {
					log.Println("TUN interface closed, shutting down...")
					return err
				}

				if os.IsNotExist(err) || os.IsPermission(err) {
					log.Printf("TUN interface error (closed or permission issue): %v", err)
					return err
				}

				log.Printf("failed to read from TUN, retrying: %v", err)
				continue
			}

			if n < 12 {
				log.Printf("invalid packet length (%d < 12)", n)
				continue
			}

			// see udp_reader.go. It's putting payload length into first 12 bytes.
			payload := packetBuffer[12 : n+12]
			parser := network.FromIPPacket(payload)
			destinationBytesErr := parser.ReadDestinationAddressBytes(destinationAddressBytes[:])
			if destinationBytesErr != nil {
				log.Printf("packet dropped: failed to read destination address bytes: %v", destinationBytesErr)
				continue
			}

			clientSession, getErr := u.sessionManager.GetByInternalIP(destinationAddressBytes[:])
			if getErr != nil {
				log.Printf("packet dropped: %s, destination host: %v", getErr, destinationAddressBytes)
				continue
			}

			encryptedPacket, encryptErr := clientSession.CryptographyService.Encrypt(packetBuffer)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, writeToUDPErr := clientSession.connectionAdapter.Write(encryptedPacket)
			if writeToUDPErr != nil {
				log.Printf("failed to send packet to %v: %v", clientSession.remoteAddrPort, writeToUDPErr)
			}
		}
	}
}

func (u *UdpTunWorker) HandleTransport() error {
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort("", u.settings.Port))
	if err != nil {
		log.Fatalf("failed to resolve udp address: %s", err)
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("failed to listen on port: %s", err)
	}
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)

	log.Printf("server listening on port %s (UDP)", u.settings.Port)

	go func() {
		<-u.ctx.Done()
		_ = conn.Close()
	}()

	dataBuf := make([]byte, network.MaxPacketLengthBytes+12)
	oobBuf := make([]byte, 1024)
	destinationAddressBuf := [4]byte{}

	for {
		select {
		case <-u.ctx.Done():
			return nil
		default:
			n, _, _, clientAddr, readFromUdpErr := conn.ReadMsgUDPAddrPort(dataBuf, oobBuf)
			if readFromUdpErr != nil {
				if u.ctx.Done() != nil {
					return nil
				}

				log.Printf("failed to read from UDP: %s", readFromUdpErr)
				continue
			}

			destinationAddressBuf = clientAddr.Addr().Unmap().As4()
			clientSession, getErr := u.sessionManager.GetByExternalIP(destinationAddressBuf[:])
			if getErr != nil || clientSession.remoteAddrPort.Port() != clientAddr.Port() {
				// Pass initial data to registration function
				regErr := u.registerClient(conn, clientAddr, dataBuf[:n])
				if regErr != nil {
					log.Printf("host %v failed registration: %v", clientAddr.Addr().AsSlice(), regErr)
					_, _ = conn.WriteToUDPAddrPort([]byte{
						byte(network.SessionReset),
					}, clientAddr)
				}
				continue
			}

			// Handle client data
			decrypted, decryptionErr := clientSession.CryptographyService.Decrypt(dataBuf[:n])
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %v", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = u.tun.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
			}
		}
	}
}

func (u *UdpTunWorker) registerClient(conn *net.UDPConn, clientAddr netip.AddrPort, initialData []byte) error {
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

	serverConfigurationManager := server_configuration2.NewManager(server_configuration2.NewServerResolver())
	_, err := serverConfigurationManager.Configuration()
	if err != nil {
		return fmt.Errorf("failed to read server configuration: %s", err)
	}

	cryptoSession, cryptoSessionErr := chacha20.NewUdpSession(h.Id(), h.ServerKey(), h.ClientKey(), true)
	if cryptoSessionErr != nil {
		return cryptoSessionErr
	}

	remoteAddr, remoteParseErr := netip.ParseAddrPort(clientAddr.String())
	if remoteParseErr != nil {
		return fmt.Errorf("invalid client address: %v", clientAddr)
	}

	u.sessionManager.Add(session{
		connectionAdapter: &network.ServerUdpAdapter{
			UdpConn:  conn,
			AddrPort: clientAddr,
		},
		remoteAddrPort:      remoteAddr,
		CryptographyService: cryptoSession,
		internalIP:          extractIPv4(internalIP),
		externalIP:          extractIPv4(clientAddr.Addr().Unmap().AsSlice()),
	})

	log.Printf("%v registered as: %v", clientAddr.Addr().As4(), internalIP)

	return nil
}

func isIPv4Mapped(ip net.IP) bool {
	// check for ::ffff:0:0/96 prefix
	return ip[0] == 0 && ip[1] == 0 && ip[2] == 0 &&
		ip[3] == 0 && ip[4] == 0 && ip[5] == 0 &&
		ip[6] == 0 && ip[7] == 0 && ip[8] == 0 &&
		ip[9] == 0 && ip[10] == 0xff && ip[11] == 0xff
}

func extractIPv4(ip net.IP) []byte {
	if len(ip) == 16 && isIPv4Mapped(ip) {
		return ip[12:16]
	}
	if len(ip) == 4 {
		return ip
	}
	return nil
}
