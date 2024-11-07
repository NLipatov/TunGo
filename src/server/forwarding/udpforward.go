package forwarding

import (
	"context"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/network"
	"etha-tunnel/network/keepalive"
	"etha-tunnel/network/packets"
	"etha-tunnel/settings/server"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

var clientAddrToInternalIP sync.Map

type UDPClient struct {
	conn *net.UDPConn
	addr *net.UDPAddr
}

func TunToUDP(tunFile *os.File, intIPToUDPClientAddr *sync.Map, intIPToSession *sync.Map, ctx context.Context) {
	conf, err := (&server.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}

	buf := make([]byte, conf.UDPSettings.MTU-12)
	sendChan := make(chan UDPClientPacket, 100_000)

	go func(sendChan chan UDPClientPacket, ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case packet := <-sendChan:
				_, err := packet.client.conn.WriteToUDP(packet.payload, packet.client.addr)
				if err != nil {
					log.Printf("failed to send packet to %s: %v", packet.client.addr, err)
				}
			}
		}
	}(sendChan, ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := tunFile.Read(buf)
			if err != nil {
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

			data := buf[:n]
			if len(data) < 1 {
				log.Printf("invalid IP data")
				continue
			}

			// Check IP version
			ipVersion := data[0] >> 4
			if ipVersion == 6 {
				// Skip IPv6 packet
				continue
			}

			header, err := packets.Parse(data)
			if err != nil {
				log.Printf("failed to parse IP header: %v", err)
				continue
			}

			destinationIP := header.GetDestinationIP().String()
			clientInfoValue, ok := intIPToUDPClientAddr.Load(destinationIP)
			if !ok {
				sourceIP := header.GetSourceIP().String()
				log.Printf("packet dropped: no conn with destination (source IP: %s, dest. IP:%s)", sourceIP, destinationIP)
				continue
			}
			clientInfo := clientInfoValue.(*UDPClient)

			// Retrieve the session for this client
			sessionValue, sessionExists := intIPToSession.Load(destinationIP)
			if !sessionExists {
				log.Printf("failed to load session for IP %s", destinationIP)
				continue
			}
			session := sessionValue.(*ChaCha20.Session)

			encryptedPacket, high, low, encryptErr := session.Encrypt(data)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			packet, packetEncodeErr := (&network.Packet{}).EncodeUDP(encryptedPacket, &ChaCha20.Nonce{
				Low:  low,
				High: high,
			})
			if packetEncodeErr != nil {
				log.Printf("packet encoding failed: %s", packetEncodeErr)
				continue
			}

			sendChan <- UDPClientPacket{client: clientInfo, payload: *packet.Payload}
		}
	}
}

type UDPClientPacket struct {
	client  *UDPClient
	payload []byte
}

func UDPToTun(listenPort string, tunFile *os.File, intIPToUDPClientAddr *sync.Map, intIPToSession *sync.Map, ctx context.Context) {
	addr, err := net.ResolveUDPAddr("udp", listenPort)
	if err != nil {
		log.Fatalf("failed to resolve udp address: %s", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("failed to listen on port: %s", err)
	}
	defer conn.Close()

	log.Printf("server listening on port udp:%s", listenPort)

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	conf, err := (&server.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}

	buf := make([]byte, conf.UDPSettings.MTU)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, clientAddr, readFromUdpErr := conn.ReadFromUDP(buf)
			if readFromUdpErr != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("failed to read from UDP: %s", readFromUdpErr)
				continue
			}

			intIPValue, exists := clientAddrToInternalIP.Load(clientAddr.String())
			if !exists {
				if len(buf[:n]) == 3 && string(buf[:n]) == "REG" {
					log.Printf("registration requested from %s", clientAddr.String())

					intIPToSession.Delete(intIPValue)
					intIPToUDPClientAddr.Delete(intIPValue)
					clientAddrToInternalIP.Delete(clientAddr.String())
					continue
				}

				// Pass initial data to registration function
				regErr := udpRegisterClient(conn, *clientAddr, buf[:n], intIPToUDPClientAddr, intIPToSession)
				if regErr != nil {
					log.Printf("%s failed registration: %s\n", clientAddr.String(), regErr)
				}
				continue
			}
			internalIP := intIPValue.(string)

			sessionValue, sessionExists := intIPToSession.Load(internalIP)
			if !sessionExists {
				log.Printf("failed to load session for IP %s", internalIP)
				continue
			}
			session := sessionValue.(*ChaCha20.Session)

			packet, err := (&network.Packet{}).DecodeUDP(buf[:n])
			if err != nil {
				log.Printf("failed to decode packet from %s: %v", clientAddr, err)
				continue
			}

			if packet.IsKeepAlive {
				kaResponse, kaErr := keepalive.GenerateUDP()
				if kaErr != nil {
					log.Printf("failed to generate keep-alive response: %s", kaErr)
				}
				_, udpWriteErr := conn.WriteToUDP(kaResponse, clientAddr)
				if udpWriteErr != nil {
					log.Printf("failed to write keep-alive response to UDP: %s", udpWriteErr)
				}
				continue
			}

			// Handle client data
			decrypted, _, _, decryptionErr := session.DecryptViaNonceBuf(*packet.Payload, *packet.Nonce)
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = tunFile.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
			}
		}
	}
}

func udpRegisterClient(conn *net.UDPConn, clientAddr net.UDPAddr, initialData []byte, intIPToUDPClientAddr *sync.Map, intIPToSession *sync.Map) error {
	log.Printf("connected: %s", clientAddr.IP.String())

	// Pass initialData and clientAddr to the handshake function
	serverSession, internalIpAddr, err := handshakeHandlers.OnClientConnectedUDP(conn, &clientAddr, initialData)
	if err != nil {
		return err
	}
	log.Printf("registered: %s", *internalIpAddr)

	// Use internal IP as key
	intIPToUDPClientAddr.Store(*internalIpAddr, &UDPClient{
		conn: conn,
		addr: &clientAddr,
	})
	intIPToSession.Store(*internalIpAddr, serverSession)
	clientAddrToInternalIP.Store(clientAddr.String(), *internalIpAddr)

	return nil
}
