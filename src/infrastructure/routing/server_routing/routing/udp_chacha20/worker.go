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
	conn    *net.UDPConn
	udpAddr *net.UDPAddr
	Session application.CryptographyService
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
	buf := make([]byte, network.MaxPacketLengthBytes+12)
	udpReader := chacha20.NewUdpReader(u.tun)
	key := [4]byte{}

	for {
		select {
		case <-u.ctx.Done():
			return nil
		default:
			n, err := udpReader.Read(buf)
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

			// manually check if it's a IPv4 packet and get 4-byte destination IP from it
			data := buf[12 : n+12]
			if data[0]>>4 != 4 {
				log.Printf("packet dropped: not IPv4")
				continue
			}
			parser := network.NewHeaderV4(data)
			destinationBytesErr := parser.ReadDestinationAddressBytes(key[:])
			if destinationBytesErr != nil {
				log.Printf("packet dropped: failed to read destination address bytes: %v", destinationBytesErr)
				continue
			}

			session, ok := u.internalIpToSession[key]
			if !ok {
				log.Printf("packet dropped: non-IPv4 dest %v", key)
				continue
			}

			encryptedPacket, encryptErr := session.Session.Encrypt(buf)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, writeToUDPErr := session.conn.WriteToUDP(encryptedPacket, session.udpAddr)
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
	key := [4]byte{}

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

			copy(key[:], clientAddr.IP.To4())
			session, ok := u.externalAddrToSession[key]
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

	session := udpClientSession{
		conn:    conn,
		udpAddr: clientAddr,
		Session: udpSession,
	}

	ip := net.ParseIP(internalIpAddr)
	key, ok := ip4key(ip)
	if !ok {
		return fmt.Errorf("invalid internal IPv4: %s", internalIpAddr)
	}
	u.internalIpToSession[key] = session

	key, ok = ip4key(clientAddr.IP)
	if !ok {
		return fmt.Errorf("invalid IPv4: %v", clientAddr.IP)
	}
	u.externalAddrToSession[key] = session

	return nil
}

func ip4key(ip net.IP) (k [4]byte, ok bool) {
	ip4 := ip.To4()
	if ip4 == nil {
		return
	}
	copy(k[:], ip4)
	return k, true
}
