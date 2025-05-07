package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/network"
	"tungo/settings"
	"tungo/settings/server_configuration"
)

type UdpTunWorker struct {
	ctx            context.Context
	tun            io.ReadWriteCloser
	settings       settings.ConnectionSettings
	sessionManager UdpWorkerSessionManager
}

func NewUdpTunWorker(
	ctx context.Context, tun io.ReadWriteCloser, settings settings.ConnectionSettings,
) application.TunWorker {
	return &UdpTunWorker{
		tun:            tun,
		ctx:            ctx,
		settings:       settings,
		sessionManager: NewUdpWorkerSessionManager(),
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

			session, ok := u.sessionManager.GetByInternalIP(destinationAddressBytes[:])
			if !ok {
				log.Printf("packet dropped: no session with destination ip %v", destinationAddressBytes)
				continue
			}

			encryptedPacket, encryptErr := session.CryptographyService.Encrypt(packetBuffer)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, writeToUDPErr := session.udpConn.WriteToUDP(encryptedPacket, session.udpAddr)
			if writeToUDPErr != nil {
				log.Printf("failed to send packet to %s: %v", session.udpAddr, writeToUDPErr)
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

	for {
		select {
		case <-u.ctx.Done():
			return nil
		default:
			n, _, _, clientAddr, readFromUdpErr := conn.ReadMsgUDP(dataBuf, oobBuf)
			if readFromUdpErr != nil {
				if u.ctx.Done() != nil {
					return nil
				}

				log.Printf("failed to read from UDP: %s", readFromUdpErr)
				continue
			}

			s, ok := u.sessionManager.GetByExternalIP(clientAddr.IP.To4())
			if !ok || s.udpAddr.Port != clientAddr.Port {
				// Pass initial data to registration function
				regErr := u.registerClient(conn, clientAddr, dataBuf[:n])
				if regErr != nil {
					log.Printf("host %s failed registration: %s", clientAddr.IP, regErr)
					_, _ = conn.WriteToUDP([]byte{
						byte(network.SessionReset),
					}, clientAddr)
				}
				continue
			}

			// Handle client data
			decrypted, decryptionErr := s.CryptographyService.Decrypt(dataBuf[:n])
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
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

func (u *UdpTunWorker) registerClient(conn *net.UDPConn, clientAddr *net.UDPAddr, initialData []byte) error {
	// Pass initialData and clientAddr to the crypto function
	h := handshake.NewHandshake()
	internalIpAddr, handshakeErr := h.ServerSideHandshake(&network.UdpAdapter{
		Conn:        *conn,
		Addr:        *clientAddr,
		InitialData: initialData,
	})
	if handshakeErr != nil {
		return handshakeErr
	}

	serverConfigurationManager := server_configuration.NewManager(server_configuration.NewServerResolver())
	_, err := serverConfigurationManager.Configuration()
	if err != nil {
		return fmt.Errorf("failed to read server configuration: %s", err)
	}

	udpSession, udpSessionErr := chacha20.NewUdpSession(h.Id(), h.ServerKey(), h.ClientKey(), true)
	if udpSessionErr != nil {
		return udpSessionErr
	}

	ip := net.ParseIP(internalIpAddr)
	u.sessionManager.Add(ClientSession{
		udpConn:             conn,
		udpAddr:             clientAddr,
		CryptographyService: udpSession,
		internalIP:          ip.To4(),
		externalIP:          clientAddr.IP.To4(),
	})

	log.Printf("%s registered as: %s", clientAddr.IP, internalIpAddr)

	return nil
}
