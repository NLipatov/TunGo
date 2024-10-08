package servertcptunforward

import (
	"context"
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
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

var S2CMutex sync.Mutex
var C2SMutex sync.Mutex

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
			packet := buf[:n]
			if len(packet) < 1 {
				log.Printf("invalid IP packet")
				continue
			}

			header, err := packets.Parse(packet)
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
				aad := session.CreateAAD(true, session.SendNonce)
				encryptedPacket, err := session.Encrypt(packet, aad)
				if err != nil {
					log.Printf("failder to encrypt a package")
					continue
				}
				err = ChaCha20.IncrementNonce(&session.SendNonce, &S2CMutex)
				if err != nil {
					log.Print(err)
					return
				}

				length := uint32(len(encryptedPacket))
				lengthBuf := make([]byte, 4)
				binary.BigEndian.PutUint32(lengthBuf, length)
				_, err = conn.Write(append(lengthBuf, encryptedPacket...))
				if err != nil {
					log.Printf("failed to send packet to client: %v", err)
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

	serverSession, extIpAddr, err := handshakeHandlers.OnClientConnected(conn)
	if err != nil {
		conn.Close()
		log.Printf("connection with %s is closed (regfail: %s)\n", conn.RemoteAddr(), err)
		return
	}
	log.Printf("registered: %s", conn.RemoteAddr())

	localIpToConn.Store(*extIpAddr, conn)
	localIpToServerSessionMap.Store(*extIpAddr, serverSession)

	handleClient(conn, tunFile, localIpToConn, localIpToServerSessionMap, extIpAddr)
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

		// Extract the length
		length := binary.BigEndian.Uint32(buf[:4])
		if length > maxPacketLengthBytes {
			log.Printf("packet too large: %d", length)
			return
		}

		// Read the encrypted packet
		_, err = io.ReadFull(conn, buf[:length])
		if err != nil {
			log.Printf("failed to read encrypted packet from client: %v", err)
			return
		}

		// Retrieve the session for this client
		sessionValue, sessionExists := localIpToSession.Load(*extIpAddr)
		if !sessionExists {
			log.Printf("failed to load session for IP %s", *extIpAddr)
			continue
		}

		session := sessionValue.(*ChaCha20.Session)

		// Create AAD for decryption using C2SCounter
		aad := session.CreateAAD(false, session.RecvNonce)

		// Decrypt the data
		packet, err := session.Decrypt(buf[:length], aad)
		if err != nil {
			log.Printf("failed to decrypt packet: %v", err)
			return
		}

		err = ChaCha20.IncrementNonce(&session.RecvNonce, &C2SMutex)
		if err != nil {
			log.Print(err)
			return
		}

		// Validate the packet (optional but recommended)
		if _, err := packets.Parse(packet); err != nil {
			log.Printf("invalid IP packet structure: %v", err)
			continue
		}

		// Write the decrypted packet to the TUN interface
		_, err = tunFile.Write(packet)
		if err != nil {
			log.Printf("failed to write to TUN: %v", err)
			return
		}
	}
}
