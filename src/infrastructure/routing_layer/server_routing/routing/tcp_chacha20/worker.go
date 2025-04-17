package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"log"
	"net"
	"os"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing_layer/server_routing/client_session"
	"tungo/settings"
)

type TcpTunWorker struct {
	sessionManager *client_session.Manager[net.Conn, net.Addr]
}

func NewTcpTunWorker() TcpTunWorker {
	return TcpTunWorker{
		sessionManager: client_session.NewManager[net.Conn, net.Addr]()}
}

func (w *TcpTunWorker) HandleTun(tunFile *os.File, ctx context.Context) {
	headerParser := network.NewBaseHeaderParser()

	buf := make([]byte, network.MaxPacketLengthBytes)
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

			internalIP := header.GetDestinationIP().String()
			session, sessionExist := w.sessionManager.Load(internalIP)
			if !sessionExist {
				log.Printf("packet dropped: no session established with host: %s", internalIP)
				continue
			}

			cryptographyService := session.Session()
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

			_, connWriteErr := session.Conn().Write(buf[:n+4+chacha20poly1305.Overhead])
			if connWriteErr != nil {
				log.Printf("failed to write to TCP: %v", connWriteErr)
				w.sessionManager.Delete(internalIP)
			}
		}
	}
}

func (w *TcpTunWorker) HandleTransport(settings settings.ConnectionSettings, tunFile *os.File, ctx context.Context) {
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
			go w.registerClient(conn, tunFile, ctx)
		}
	}
}

func (w *TcpTunWorker) registerClient(conn net.Conn, tunFile *os.File, ctx context.Context) {
	log.Printf("connected: %s", conn.RemoteAddr())
	h := chacha20.NewHandshake()
	internalIP, handshakeErr := h.ServerSideHandshake(&network.TcpAdapter{
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
	_, ipCollision := w.sessionManager.Load(*internalIP)
	if ipCollision {
		log.Printf("connection closed: %s (internal ip %s already in use)\n", conn.RemoteAddr(), *internalIP)
		_ = conn.Close()
	}

	w.sessionManager.Store(client_session.NewSessionImpl(conn, *internalIP, conn.RemoteAddr(), cryptographyService))

	w.handleClient(conn, tunFile, internalIP, ctx)
}

func (w *TcpTunWorker) handleClient(conn net.Conn, tunFile *os.File, internalIP *string, ctx context.Context) {
	defer func() {
		w.sessionManager.Delete(*internalIP)
		w.sessionManager.Delete(conn.RemoteAddr().String())
		_ = conn.Close()
		log.Printf("disconnected: %s", conn.RemoteAddr())
	}()

	buf := make([]byte, network.MaxPacketLengthBytes)
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
			sessionValue, sessionExists := w.sessionManager.Load(*internalIP)
			if !sessionExists {
				log.Printf("failed to load session for IP %s", *internalIP)
				continue
			}

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buf[:4])
			if length < 4 || length > network.MaxPacketLengthBytes {
				log.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
				continue
			}

			decrypted, decryptionErr := sessionValue.Session().Decrypt(buf[:length])
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
