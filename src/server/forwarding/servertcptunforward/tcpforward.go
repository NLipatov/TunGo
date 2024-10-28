package servertcptunforward

import (
	"context"
	"encoding/binary"
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

func TunToTCP(tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
	buf := make([]byte, IPPacketMaxSizeBytes)
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

func TCPToTun(listenPort string, tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
	listener, err := net.Listen("tcp", listenPort)
	if err != nil {
		log.Printf("failed to listen on port %s: %v", listenPort, err)
	}
	defer func() {
		_ = listener.Close()
	}()
	log.Printf("server listening on port tcp:%s", listenPort)

	//using this goroutine to 'unblock' Listener.Accept blocking-call
	go func() {
		<-ctx.Done() //blocks till ctx.Done signal comes in
		_ = listener.Close()
		return
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, listenErr := listener.Accept()
			if ctx.Err() != nil {
				log.Printf("exiting Accept loop: %s", ctx.Err())
				return
			}
			if listenErr != nil {
				log.Printf("failed to accept connection: %v", listenErr)
				continue
			}
			go registerClient(conn, tunFile, localIpMap, localIpToSessionMap, ctx)
		}
	}
}

func registerClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToServerSessionMap *sync.Map, ctx context.Context) {
	log.Printf("connected: %s", conn.RemoteAddr())

	serverSession, internalIpAddr, err := handshakeHandlers.OnClientConnected(conn)
	if err != nil {
		_ = conn.Close()
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

	handleClient(conn, tunFile, localIpToConn, localIpToServerSessionMap, internalIpAddr, ctx)
}

func handleClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToSession *sync.Map, extIpAddr *string, ctx context.Context) {
	defer func() {
		localIpToConn.Delete(*extIpAddr)
		localIpToSession.Delete(*extIpAddr)
		_ = conn.Close()
		log.Printf("disconnected: %s", conn.RemoteAddr())
	}()

	buf := make([]byte, IPPacketMaxSizeBytes)
	for {
		select {
		case <-ctx.Done():
			return
		default:
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

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buf[:4])
			if length < 4 || length > IPPacketMaxSizeBytes {
				log.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
				continue
			}

			packet, err := (&network.Packet{}).Decode(buf[:length])

			//shortcut for keep alive response case
			if packet.Length == 9 && keepalive.IsKeepAlive(packet.Payload) {
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

			decrypted, decryptionErr := session.Decrypt(buf[:length])
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = tunFile.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
