package server

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"tungo/crypto/chacha20"
	"tungo/network"
	"tungo/network/ip"
	"tungo/settings"
	"tungo/settings/server"
)

type UDPClient struct {
	conn *net.UDPConn
	addr *net.UDPAddr
}

type UdpTunWorker struct {
	ctx                    context.Context
	tun                    *os.File
	settings               settings.ConnectionSettings
	clientAddrToInternalIP sync.Map
	intIPToUDPClient       *sync.Map
	intIPToSession         *sync.Map
}

func NewUdpTunWorker(ctx context.Context, tun *os.File, settings settings.ConnectionSettings, intIPToUDPClient *sync.Map, intIPToSession *sync.Map) UdpTunWorker {
	return UdpTunWorker{
		tun:              tun,
		ctx:              ctx,
		settings:         settings,
		intIPToUDPClient: intIPToUDPClient,
		intIPToSession:   intIPToSession,
	}
}

func (u *UdpTunWorker) TunToUDP() {
	headerParser := newBaseIpHeaderParser()

	buf := make([]byte, ip.MaxPacketLengthBytes+12)
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
			clientInfoValue, ok := u.intIPToUDPClient.Load(destinationIP)
			if !ok {
				if destinationIP == "" || destinationIP == "<nil>" {
					log.Printf("packet dropped: no dest. IP specified in IP packet header")
					continue
				}

				log.Printf("packet dropped: no connection with destination (source IP: %s, dest. IP:%s)", sourceIP, destinationIP)
				continue
			}
			clientInfo := clientInfoValue.(*UDPClient)

			// Retrieve the session for this client
			sessionValue, sessionExists := u.intIPToSession.Load(destinationIP)
			if !sessionExists {
				log.Printf("failed to load session for IP %s", destinationIP)
				continue
			}
			session := sessionValue.(*chacha20.DefaultUdpSession)

			encryptedPacket, encryptErr := session.Encrypt(buf)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			_, writeToUDPErr := clientInfo.conn.WriteToUDP(encryptedPacket, clientInfo.addr)
			if err != nil {
				log.Printf("failed to send packet to %s: %v", clientInfo.addr, writeToUDPErr)
			}
		}
	}
}

func (u *UdpTunWorker) UDPToTun() {
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

	dataBuf := make([]byte, ip.MaxPacketLengthBytes+12)
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

			intIPValue, exists := u.clientAddrToInternalIP.Load(clientAddr.String())
			if !exists {
				u.intIPToSession.Delete(intIPValue)
				u.intIPToUDPClient.Delete(intIPValue)
				u.clientAddrToInternalIP.Delete(clientAddr.String())

				// Pass initial data to registration function
				regErr := u.udpRegisterClient(conn, *clientAddr, dataBuf[:n], u.intIPToUDPClient, u.intIPToSession)
				if regErr != nil {
					log.Printf("%s failed registration: %s\n", clientAddr.String(), regErr)
				}
				continue
			}
			internalIP := intIPValue.(string)

			sessionValue, sessionExists := u.intIPToSession.Load(internalIP)
			if !sessionExists {
				log.Printf("failed to load session for IP %s", internalIP)
				continue
			}
			session := sessionValue.(*chacha20.DefaultUdpSession)

			// Handle client data
			decrypted, decryptionErr := session.Decrypt(dataBuf[:n])
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

func (u *UdpTunWorker) udpRegisterClient(conn *net.UDPConn, clientAddr net.UDPAddr, initialData []byte, intIPToUDPClientAddr *sync.Map, intIPToSession *sync.Map) error {
	// Pass initialData and clientAddr to the crypto function
	h := chacha20.NewHandshake()
	internalIpAddr, handshakeErr := h.ServerSideHandshake(&network.UdpAdapter{
		Conn:        *conn,
		Addr:        clientAddr,
		InitialData: initialData,
	})
	if handshakeErr != nil {
		return handshakeErr
	}
	log.Printf("%s registered as: %s", clientAddr.String(), *internalIpAddr)

	conf, confErr := (&server.Conf{}).Read()
	if confErr != nil {
		return confErr
	}

	udpSession, tcpSessionErr := chacha20.NewUdpSession(h.Id(), h.ServerKey(), h.ClientKey(), true, conf.UDPNonceRingBufferSize)
	if tcpSessionErr != nil {
		log.Printf("%s failed registration: %s", conn.RemoteAddr(), tcpSessionErr)
	}

	// Use internal IP as key
	intIPToUDPClientAddr.Store(*internalIpAddr, &UDPClient{
		conn: conn,
		addr: &clientAddr,
	})
	intIPToSession.Store(*internalIpAddr, udpSession)
	u.clientAddrToInternalIP.Store(clientAddr.String(), *internalIpAddr)

	return nil
}
