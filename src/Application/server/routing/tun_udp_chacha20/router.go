package tun_udp_chacha20

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"tungo/Application/boundary"
	"tungo/Application/crypto/chacha20"
	"tungo/Application/crypto/chacha20/handshake"
	"tungo/Domain"
	"tungo/Domain/settings"
	"tungo/Infrastructure/network"
	"tungo/Infrastructure/network/packets"
)

type UDPRouter struct {
	settings               settings.ConnectionSettings
	tun                    boundary.TunAdapter
	clientAddrToInternalIP *sync.Map
	intIPToUDPClientAddr   *sync.Map
	intIPToSession         *sync.Map
	ctx                    context.Context
	err                    error
}

func (r *UDPRouter) TunToUDP() {
	buf := make([]byte, Domain.IPPacketMaxSizeBytes)
	sendChan := make(chan clientPacket, 100_000)

	go func(sendChan chan clientPacket, ctx context.Context) {
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
	}(sendChan, r.ctx)

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			n, err := r.tun.Read(buf)
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
			clientInfoValue, ok := r.intIPToUDPClientAddr.Load(destinationIP)
			if !ok {
				sourceIP := header.GetSourceIP().String()
				log.Printf("packet dropped: no conn with destination (source IP: %s, dest. IP:%s)", sourceIP, destinationIP)
				continue
			}
			clientInfo := clientInfoValue.(*clientData)

			// Retrieve the session for this client
			sessionValue, sessionExists := r.intIPToSession.Load(destinationIP)
			if !sessionExists {
				log.Printf("failed to load session for IP %s", destinationIP)
				continue
			}
			session := sessionValue.(*chacha20.Session)

			encryptedPacket, nonce, encryptErr := session.Encrypt(data)
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			packet, packetEncodeErr := (&chacha20.UDPEncoder{}).Encode(encryptedPacket, nonce)
			if packetEncodeErr != nil {
				log.Printf("packet encoding failed: %s", packetEncodeErr)
				continue
			}

			sendChan <- clientPacket{client: clientInfo, payload: *packet.Payload}
		}
	}
}
func (r *UDPRouter) UDPToTun() {
	addr, err := net.ResolveUDPAddr("udp", r.settings.Port)
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

	log.Printf("server listening on port %s (UDP)", r.settings.Port)

	go func() {
		<-r.ctx.Done()
		_ = conn.Close()
	}()

	buf := make([]byte, Domain.IPPacketMaxSizeBytes)
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			n, clientAddr, readFromUdpErr := conn.ReadFromUDP(buf)
			if readFromUdpErr != nil {
				if r.ctx.Err() != nil {
					return
				}
				log.Printf("failed to read from UDP: %s", readFromUdpErr)
				continue
			}

			intIPValue, exists := r.clientAddrToInternalIP.Load(clientAddr.String())
			if !exists {
				r.intIPToSession.Delete(intIPValue)
				r.intIPToUDPClientAddr.Delete(intIPValue)
				r.clientAddrToInternalIP.Delete(clientAddr.String())

				// Pass initial data to registration function
				regErr := r.udpRegisterClient(conn, *clientAddr, buf[:n])
				if regErr != nil {
					log.Printf("%s failed registration: %s\n", clientAddr.String(), regErr)
				}
				continue
			}
			internalIP := intIPValue.(string)

			sessionValue, sessionExists := r.intIPToSession.Load(internalIP)
			if !sessionExists {
				log.Printf("failed to load session for IP %s", internalIP)
				continue
			}
			session := sessionValue.(*chacha20.Session)

			// Handle client data
			decrypted, _, decryptionErr := session.DecryptViaNonceBuf(buf[:n])
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = r.tun.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
			}
		}
	}
}

func (r *UDPRouter) udpRegisterClient(conn *net.UDPConn, clientAddr net.UDPAddr, initialData []byte) error {
	// Pass initialData and clientAddr to the crypto function
	serverSession, internalIpAddr, err := handshake.OnClientConnected(&network.UdpAdapter{
		Conn:        *conn,
		Addr:        clientAddr,
		InitialData: initialData,
	})
	if err != nil {
		return err
	}
	log.Printf("%s registered as: %s", clientAddr.String(), *internalIpAddr)

	// Use internal IP as key
	r.intIPToUDPClientAddr.Store(*internalIpAddr, &clientData{
		conn: conn,
		addr: &clientAddr,
	})
	r.intIPToSession.Store(*internalIpAddr, serverSession)
	r.clientAddrToInternalIP.Store(clientAddr.String(), *internalIpAddr)

	return nil
}
