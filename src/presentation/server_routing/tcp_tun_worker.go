package server_routing

import (
	"context"
	"encoding/binary"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
	"tungo/infrastructure/network/ip"
	"tungo/settings"
)

type TcpTunWorker struct {
}

func NewTcpTunWorker() TcpTunWorker {
	return TcpTunWorker{}
}

func (w *TcpTunWorker) TunToTCP(tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
	headerParser := ip.NewBaseHeaderParser()

	buf := make([]byte, network.IPPacketMaxSizeBytes)
	reader := chacha20.NewTcpReader(tunFile)
	encoder := chacha20.NewDefaultTCPEncoder()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := reader.Read(buf)
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
			data := buf[4 : n+4]
			if len(data) < 1 {
				log.Printf("invalid IP data")
				continue
			}

			header, headerErr := headerParser.Parse(data)
			if headerErr != nil {
				log.Printf("failed to parse IP header: %v", headerErr)
				continue
			}

			destinationIP := header.GetDestinationIP().String()
			v, ok := localIpMap.Load(destinationIP)
			if ok {
				conn := v.(net.Conn)
				sessionValue, sessionExists := localIpToSessionMap.Load(destinationIP)
				if !sessionExists {
					log.Printf("failed to load cryptographyService")
					continue
				}
				cryptographyService := sessionValue.(*chacha20.TcpCryptographyService)
				_, encryptErr := cryptographyService.Encrypt(buf[4 : n+4])
				if encryptErr != nil {
					log.Printf("failder to encrypt a package: %s", encryptErr)
					continue
				}

				encodingErr := encoder.Encode(buf[:n+4+chacha20poly1305.Overhead])
				if encodingErr != nil {
					log.Printf("failed to encode packet: %v", encodingErr)
					continue
				}

				_, connWriteErr := conn.Write(buf[:n+4+chacha20poly1305.Overhead])
				if connWriteErr != nil {
					log.Printf("failed to write to TCP: %v", connWriteErr)
					localIpMap.Delete(destinationIP)
					localIpToSessionMap.Delete(destinationIP)
				}
			}
		}
	}
}

func (w *TcpTunWorker) TCPToTun(settings settings.ConnectionSettings, tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
	listener, err := net.Listen("tcp", net.JoinHostPort("", settings.Port))
	if err != nil {
		log.Printf("failed to listen on port %s: %v", settings.Port, err)
	}
	defer func() {
		_ = listener.Close()
	}()
	log.Printf("server listening on port %s (TCP)", settings.Port)

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
			go w.registerClient(conn, tunFile, localIpMap, localIpToSessionMap, ctx)
		}
	}
}

func (w *TcpTunWorker) registerClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToServerSessionMap *sync.Map, ctx context.Context) {
	log.Printf("connected: %s", conn.RemoteAddr())
	h := chacha20.NewHandshake()
	internalIpAddr, handshakeErr := h.ServerSideHandshake(&network.TcpAdapter{
		Conn: conn,
	})
	if handshakeErr != nil {
		_ = conn.Close()
		log.Printf("connection closed: %s (regfail: %s)\n", conn.RemoteAddr(), handshakeErr)
		return
	}
	log.Printf("registered: %s", conn.RemoteAddr())

	cryptographyService, cryptographyServiceErr := chacha20.NewTcpCryptographyService(h.Id(), h.ServerKey(), h.ClientKey(), true)
	if cryptographyServiceErr != nil {
		_ = conn.Close()
		log.Printf("connection closed: %s (regfail: %s)\n", conn.RemoteAddr(), cryptographyServiceErr)
	}

	// Prevent IP spoofing
	_, ipCollision := localIpToConn.Load(*internalIpAddr)
	if ipCollision {
		log.Printf("connection closed: %s (internal ip %s already in use)\n", conn.RemoteAddr(), *internalIpAddr)
		_ = conn.Close()
	}

	localIpToConn.Store(*internalIpAddr, conn)
	localIpToServerSessionMap.Store(*internalIpAddr, cryptographyService)

	w.handleClient(conn, tunFile, localIpToConn, localIpToServerSessionMap, internalIpAddr, ctx)
}

func (w *TcpTunWorker) handleClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToSession *sync.Map, extIpAddr *string, ctx context.Context) {
	defer func() {
		localIpToConn.Delete(*extIpAddr)
		localIpToSession.Delete(*extIpAddr)
		_ = conn.Close()
		log.Printf("disconnected: %s", conn.RemoteAddr())
	}()

	buf := make([]byte, network.IPPacketMaxSizeBytes)
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

			session := sessionValue.(*chacha20.TcpCryptographyService)

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buf[:4])
			if length < 4 || length > network.IPPacketMaxSizeBytes {
				log.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
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
