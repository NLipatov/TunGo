package servertcptunforward

import (
	"context"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/network"
	"etha-tunnel/network/keepalive"
	"etha-tunnel/network/packets"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

const (
	maxPacketLengthBytes = 65535
)

func ToTCP(tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
	buf := make([]byte, maxPacketLengthBytes)
	for {
		select {
		case <-ctx.Done():
			log.Println("server is shutting down.")
			return
		default:
			n, err := tunFile.Read(buf)
			if err != nil {
				log.Printf("failed to read from TUN: %v", err)
				continue
			}
			data := buf[:n]
			if len(data) < 1 {
				log.Printf("invalid IP data")
				continue
			}

			header, err := packets.Parse(data)
			if err != nil {
				log.Printf("failed to parse a IPv4 header")
				continue
			}
			destinationIP := header.GetDestinationIP().String()
			v, ok := localIpMap.Load(destinationIP)
			if ok {
				conn := v.(net.Conn)
				sessionValue, sessionExists := localIpToSessionMap.Load(destinationIP)
				if !sessionExists {
					log.Printf("failed to load session")
					continue
				}
				session := sessionValue.(*ChaCha20.Session)
				encryptedPacket, encryptErr := session.Encrypt(data)
				if encryptErr != nil {
					log.Printf("failder to encrypt a package: %s", encryptErr)
					continue
				}

				packet, packetEncodeErr := (&network.Packet{}).Encode(encryptedPacket)
				if packetEncodeErr != nil {
					log.Printf("packet encoding failed: %s", packetEncodeErr)
				}

				_, connWriteErr := conn.Write(packet.Payload)
				if connWriteErr != nil {
					log.Printf("failed to write to TCP: %v", connWriteErr)
					localIpMap.Delete(destinationIP)
					localIpToSessionMap.Delete(destinationIP)
				}
			}
		}
	}
}

func ToTun(listenPort string, tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
	listener, err := net.Listen("tcp", listenPort)
	if err != nil {
		log.Printf("failed to listen on port %s: %v", listenPort, err)
	}
	defer listener.Close()
	log.Printf("server listening on port %s", listenPort)

	//using this goroutine to 'unblock' Listener.Accept blocking-call
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			log.Println("Server is shutting down.")
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("failed to accept connection: %v", err)
				continue
			}
			go registerClient(conn, tunFile, localIpMap, localIpToSessionMap)
		}
	}
}

func registerClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToServerSessionMap *sync.Map) {
	log.Printf("connected: %s", conn.RemoteAddr())

	serverSession, internalIpAddr, err := handshakeHandlers.OnClientConnected(conn)
	if err != nil {
		conn.Close()
		log.Printf("conn closed: %s (regfail: %s)\n", conn.RemoteAddr(), err)
		return
	}
	log.Printf("registered: %s", conn.RemoteAddr())

	// Prevent IP spoofing
	_, ipCollision := localIpToConn.Load(*internalIpAddr)
	if ipCollision {
		log.Printf("conn closed: %s (internal ip %s already in use)\n", conn.RemoteAddr(), *internalIpAddr)
		_ = conn.Close()
	}

	localIpToConn.Store(*internalIpAddr, conn)
	localIpToServerSessionMap.Store(*internalIpAddr, serverSession)

	handleClient(conn, tunFile, localIpToConn, localIpToServerSessionMap, internalIpAddr)
}

func handleClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToSession *sync.Map, extIpAddr *string) {
	defer func() {
		localIpToConn.Delete(*extIpAddr)
		localIpToSession.Delete(*extIpAddr)
		conn.Close()
		log.Printf("disconnected: %s", conn.RemoteAddr())
	}()

	buf := make([]byte, maxPacketLengthBytes)
	for {
		// Read the length of the encrypted packet (4 bytes)
		_, err := io.ReadFull(conn, buf[:4])
		if err != nil {
			if err != io.EOF {
				log.Printf("failed to read from client: %v", err)
			}
			return
		}

		// Retrieve the session for this client
		sessionValue, sessionExists := localIpToSession.Load(*extIpAddr)
		if !sessionExists {
			log.Printf("failed to load session for IP %s", *extIpAddr)
			continue
		}

		session := sessionValue.(*ChaCha20.Session)

		packet, err := (&network.Packet{}).Decode(conn, buf, session)
		if err != nil {
			log.Println(err)
			continue
		}

		if packet.IsKeepAlive {
			kaResponse, kaErr := keepalive.Generate()
			if kaErr != nil {
				log.Printf("failed to generate keep-alive response: %s", kaErr)
			}
			_, tcpWriteErr := conn.Write(kaResponse)
			if tcpWriteErr != nil {
				log.Printf("failed to write keep-alive response to TCP: %s", tcpWriteErr)
			}
			continue
		}

		// Validate the packet (optional but recommended)
		if _, err := packets.Parse(packet.Payload); err != nil {
			log.Printf("invalid IP packet structure: %v", err)
			continue
		}

		// Write the decrypted packet to the TUN interface
		_, err = tunFile.Write(packet.Payload)
		if err != nil {
			log.Printf("failed to write to TUN: %v", err)
			return
		}
	}
}
