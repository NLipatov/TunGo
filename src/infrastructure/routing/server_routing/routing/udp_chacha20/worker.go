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

type udpClientSession struct {
	conn                   *net.UDPConn
	remoteUdpAddr          *net.UDPAddr
	Session                application.CryptographyService
	internalIP, externalIP []byte
}

type UdpTunWorker struct {
	ctx                   context.Context
	tun                   io.ReadWriteCloser
	settings              settings.ConnectionSettings
	internalIpToSession   map[[4]byte]udpClientSession
	externalAddrToSession map[[4]byte]udpClientSession
}

func NewUdpTunWorker(
	ctx context.Context, tun io.ReadWriteCloser, settings settings.ConnectionSettings,
) application.TunWorker {
	return &UdpTunWorker{
		tun:                   tun,
		ctx:                   ctx,
		settings:              settings,
		internalIpToSession:   make(map[[4]byte]udpClientSession),
		externalAddrToSession: make(map[[4]byte]udpClientSession),
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
			parser := network.NewHeaderV4(payload)
			destinationBytesErr := parser.ReadDestinationAddressBytes(destinationAddressBytes[:])
			if destinationBytesErr != nil {
				log.Printf("packet dropped: failed to read destination address bytes: %v", destinationBytesErr)
				continue
			}

			session, ok := u.internalIpToSession[destinationAddressBytes]
			if !ok {
				log.Printf("packet dropped: non-IPv4 dest %v", destinationAddressBytes)
				continue
			}

			encryptedPacket, encryptErr := session.Session.Encrypt(packetBuffer)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, writeToUDPErr := session.conn.WriteToUDP(encryptedPacket, session.remoteUdpAddr)
			if writeToUDPErr != nil {
				log.Printf("failed to send packet to %s: %v", session.remoteUdpAddr, writeToUDPErr)
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
	destinationAddressBytes := [4]byte{}

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

			copy(destinationAddressBytes[:], clientAddr.IP.To4())
			session, ok := u.externalAddrToSession[destinationAddressBytes]
			if !ok {
				// Pass initial data to registration function
				regErr := u.udpRegisterClient(conn, clientAddr, dataBuf[:n])
				if regErr != nil {
					log.Printf("%s failed registration: %s\n", clientAddr.IP, regErr)
					_, _ = conn.WriteToUDP([]byte{
						byte(network.SessionReset),
					}, clientAddr)
				}
				continue
			}

			// Handle client data
			decrypted, decryptionErr := session.Session.Decrypt(dataBuf[:n])
			if decryptionErr != nil {
				delete(u.externalAddrToSession, [4]byte(session.externalIP))
				delete(u.internalIpToSession, [4]byte(session.internalIP))
				log.Printf("failed to decrypt data: %s", decryptionErr)
				_, _ = conn.WriteToUDP([]byte{
					byte(network.SessionReset),
				}, clientAddr)
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
	h := handshake.NewHandshake()
	internalIpAddr, handshakeErr := h.ServerSideHandshake(&network.UdpAdapter{
		Conn:        *conn,
		Addr:        *clientAddr,
		InitialData: initialData,
	})
	if handshakeErr != nil {
		return handshakeErr
	}
	log.Printf("%s registered as: %s", clientAddr.IP, internalIpAddr)

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
	session := udpClientSession{
		conn:          conn,
		remoteUdpAddr: clientAddr,
		Session:       udpSession,
		internalIP:    ip.To4(),
		externalIP:    clientAddr.IP.To4(),
	}

	u.internalIpToSession[[4]byte(session.internalIP)] = session
	u.externalAddrToSession[[4]byte(session.externalIP)] = session

	return nil
}
