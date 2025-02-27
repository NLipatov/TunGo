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
	"tungo/network/packets"
	"tungo/network/pipes"
	"tungo/settings"
)

func TunToTCP(tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
	buf := make([]byte, network.IPPacketMaxSizeBytes)
	encoder := chacha20.DefaultTCPEncoder{}
	pipeMap := sync.Map{}

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
			if len(buf[:n]) < 1 {
				log.Printf("invalid IP data")
				continue
			}

			header, err := packets.Parse(buf[:n])
			if err != nil {
				log.Printf("failed to parse a IPv4 header")
				continue
			}
			destinationIP := header.GetDestinationIP().String()
			v, ok := localIpMap.Load(destinationIP)
			if ok {
				var pipe pipes.Pipe
				pipeValue, pipeExist := pipeMap.Load(destinationIP)
				if !pipeExist {
					conn := v.(net.Conn)
					sessionValue, sessionExists := localIpToSessionMap.Load(destinationIP)
					if !sessionExists {
						log.Printf("failed to load session")
						continue
					}
					session := sessionValue.(*chacha20.TcpSession)

					pipe = pipes.NewEncryptionPipe(
						pipes.NewTcpEncodingPipe(
							pipes.NewDefaultPipe(conn), &encoder),
						session)
					pipeMap.Store(destinationIP, pipe)
				} else {
					pipe = pipeValue.(*pipes.EncryptionPipe)
				}

				passErr := pipe.Pass(buf[:n])
				if passErr != nil {
					pipeMap.Delete(destinationIP)
					localIpMap.Delete(destinationIP)
					localIpToSessionMap.Delete(destinationIP)
				}
			}
		}
	}
}

func TCPToTun(settings settings.ConnectionSettings, tunFile *os.File, localIpMap *sync.Map, localIpToSessionMap *sync.Map, ctx context.Context) {
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
			go registerClient(conn, tunFile, localIpMap, localIpToSessionMap, ctx)
		}
	}
}

func registerClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToServerSessionMap *sync.Map, ctx context.Context) {
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

	tcpSession, tcpSessionErr := chacha20.NewTcpSession(h.Id(), h.ServerKey(), h.ClientKey(), true)
	if tcpSessionErr != nil {
		_ = conn.Close()
		log.Printf("connection closed: %s (regfail: %s)\n", conn.RemoteAddr(), tcpSessionErr)
	}

	// Prevent IP spoofing
	_, ipCollision := localIpToConn.Load(*internalIpAddr)
	if ipCollision {
		log.Printf("connection closed: %s (internal ip %s already in use)\n", conn.RemoteAddr(), *internalIpAddr)
		_ = conn.Close()
	}

	localIpToConn.Store(*internalIpAddr, conn)
	localIpToServerSessionMap.Store(*internalIpAddr, tcpSession)

	handleClient(conn, tunFile, localIpToConn, localIpToServerSessionMap, internalIpAddr, ctx)
}

func handleClient(conn net.Conn, tunFile *os.File, localIpToConn *sync.Map, localIpToSession *sync.Map, extIpAddr *string, ctx context.Context) {
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

			session := sessionValue.(*chacha20.TcpSession)
			pipe := pipes.
				NewTcpFrameBufferReadPipe(pipes.
					NewReaderPipe(pipes.
						NewDecryptionPipe(pipes.
							NewDefaultPipe(tunFile), session), conn))
			passErr := pipe.Pass(buf[:4])
			if passErr != nil {
				log.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
