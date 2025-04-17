package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing_layer/server_routing/client_session"
	"tungo/settings"
	"tungo/settings/server_configuration"
)

type UDPClient struct {
	conn *net.UDPConn
	addr *net.UDPAddr
}

type UdpTunWorker struct {
	ctx            context.Context
	tun            *os.File
	settings       settings.ConnectionSettings
	sessionManager *client_session.Manager[*net.UDPConn, *net.UDPAddr]
}

func NewUdpTunWorker(ctx context.Context, tun *os.File, settings settings.ConnectionSettings) UdpTunWorker {
	return UdpTunWorker{
		tun:            tun,
		ctx:            ctx,
		settings:       settings,
		sessionManager: client_session.NewManager[*net.UDPConn, *net.UDPAddr](),
	}
}

func (u *UdpTunWorker) HandleTun() {
	headerParser := network.NewBaseHeaderParser()

	buf := make([]byte, network.MaxPacketLengthBytes+12)
	udpReader := chacha20.NewUdpReader(u.tun)

	for {
		select {
		case <-u.ctx.Done():
			return
		default:
			n, err := udpReader.Read(buf)
			if err != nil {
				if u.ctx.Done() != nil {
					return
				}

				if err == io.EOF {
					log.Println("TUN interface closed, shutting down...")
					return
				}

				if os.IsNotExist(err) || os.IsPermission(err) {
					log.Printf("TUN interface error (closed or permission issue): %v", err)
					return
				}

				log.Printf("failed to read from TUN, retrying: %v", err)
				continue
			}

			if n < 12 {
				log.Printf("invalid packet length (%d < 12)", n)
				continue
			}

			header, headerErr := headerParser.Parse(buf[12 : n+12])
			if headerErr != nil {
				log.Printf("failed to parse IP header: %v", headerErr)
				continue
			}

			destinationIP := header.GetDestinationIP().String()
			sourceIP := header.GetSourceIP().String()
			udpClientSession, ok := u.sessionManager.Load(destinationIP)
			if !ok {
				if destinationIP == "" || destinationIP == "<nil>" {
					log.Printf("packet dropped: no dest. IP specified in IP packet header")
					continue
				}

				log.Printf("packet dropped: no connection with destination (source IP: %s, dest. IP:%s)", sourceIP, destinationIP)
				continue
			}

			encryptedPacket, encryptErr := udpClientSession.Session().Encrypt(buf)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, writeToUDPErr := udpClientSession.Conn().WriteToUDP(encryptedPacket, udpClientSession.Addr())
			if writeToUDPErr != nil {
				log.Printf("failed to send packet to %s: %v", udpClientSession.Addr(), writeToUDPErr)
			}
		}
	}
}

func (u *UdpTunWorker) HandleTransport() {
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort("", u.settings.Port))
	if err != nil {
		log.Fatalf("failed to resolve udp address: %s", err)
		return
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
			return
		default:
			n, _, _, clientAddr, readFromUdpErr := conn.ReadMsgUDP(dataBuf, oobBuf)
			if readFromUdpErr != nil {
				if u.ctx.Done() != nil {
					return
				}

				log.Printf("failed to read from UDP: %s", readFromUdpErr)
				continue
			}

			udpClientSession, exists := u.sessionManager.Load(clientAddr.String())
			if !exists {
				// Pass initial data to registration function
				regErr := u.udpRegisterClient(conn, clientAddr, dataBuf[:n])
				if regErr != nil {
					log.Printf("%s failed registration: %s\n", clientAddr.String(), regErr)
					_, _ = conn.WriteToUDP([]byte{
						byte(network.SessionReset),
					}, clientAddr)
				}
				continue
			}

			// Handle client data
			decrypted, decryptionErr := udpClientSession.Session().Decrypt(dataBuf[:n])
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

func (u *UdpTunWorker) udpRegisterClient(conn *net.UDPConn, clientAddr *net.UDPAddr, initialData []byte) error {
	// Pass initialData and clientAddr to the crypto function
	h := chacha20.NewHandshake()
	internalIpAddr, handshakeErr := h.ServerSideHandshake(&network.UdpAdapter{
		Conn:        *conn,
		Addr:        *clientAddr,
		InitialData: initialData,
	})
	if handshakeErr != nil {
		return handshakeErr
	}
	log.Printf("%s registered as: %s", clientAddr.String(), *internalIpAddr)

	serverConfigurationManager := server_configuration.NewManager()
	serverConf, err := serverConfigurationManager.Configuration()
	if err != nil {
		return fmt.Errorf("failed to read server configuration: %s", err)
	}

	udpSession, udpSessionErr := chacha20.NewUdpSession(h.Id(), h.ServerKey(), h.ClientKey(), true, serverConf.UDPNonceRingBufferSize)
	if udpSessionErr != nil {
		return udpSessionErr
	}

	u.sessionManager.Store(client_session.NewSessionImpl(conn, *internalIpAddr, clientAddr, udpSession))

	return nil
}
