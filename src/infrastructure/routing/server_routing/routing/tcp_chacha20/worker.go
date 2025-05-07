package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

type TcpTunWorker struct {
	ctx            context.Context
	tunFile        io.ReadWriteCloser
	settings       settings.ConnectionSettings
	sessionManager session_management.WorkerSessionManager[session]
}

func NewTcpTunWorker(
	ctx context.Context, tunFile io.ReadWriteCloser, settings settings.ConnectionSettings,
) application.TunWorker {
	return &TcpTunWorker{
		ctx:            ctx,
		tunFile:        tunFile,
		settings:       settings,
		sessionManager: session_management.NewDefaultWorkerSessionManager[session]()}
}

func (w *TcpTunWorker) HandleTun() error {
	buf := make([]byte, network.MaxPacketLengthBytes)
	reader := chacha20.NewTcpReader(w.tunFile)
	encoder := chacha20.NewDefaultTCPEncoder()

	destinationAddressBytes := [4]byte{}

	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			n, err := reader.Read(buf)
			if err != nil {
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
			data := buf[4 : n+4]
			if len(data) < 1 {
				log.Printf("invalid IP data")
				continue
			}

			parser := network.FromIPPacket(data)
			destinationBytesErr := parser.ReadDestinationAddressBytes(destinationAddressBytes[:])
			if destinationBytesErr != nil {
				log.Printf("packet dropped: failed to read destination address bytes: %v", destinationBytesErr)
				continue
			}

			clientSession, getErr := w.sessionManager.GetByInternalIP(destinationAddressBytes[:])
			if getErr != nil {
				log.Printf("packet dropped: %s, destination host: %v", getErr, destinationAddressBytes)
				continue
			}

			_, encryptErr := clientSession.CryptographyService.Encrypt(buf[4 : n+4])
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %s", encryptErr)
				continue
			}

			encodingErr := encoder.Encode(buf[:n+4+chacha20poly1305.Overhead])
			if encodingErr != nil {
				log.Printf("failed to encode packet: %v", encodingErr)
				continue
			}

			_, connWriteErr := clientSession.conn.Write(buf[:n+4+chacha20poly1305.Overhead])
			if connWriteErr != nil {
				log.Printf("failed to write to TCP: %v", connWriteErr)
				w.sessionManager.Delete(clientSession)
			}
		}
	}
}

func (w *TcpTunWorker) HandleTransport() error {
	listener, err := net.Listen("tcp", net.JoinHostPort("", w.settings.Port))
	if err != nil {
		log.Printf("failed to listen on port %s: %v", w.settings.Port, err)
	}
	defer func() {
		_ = listener.Close()
	}()
	log.Printf("server listening on port %s (TCP)", w.settings.Port)

	//using this goroutine to 'unblock' Listener.Accept blocking-call
	go func() {
		<-w.ctx.Done() //blocks till ctx.Done signal comes in
		_ = listener.Close()
		return
	}()

	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			conn, listenErr := listener.Accept()
			if w.ctx.Err() != nil {
				log.Printf("exiting Accept loop: %s", w.ctx.Err())
				return err
			}
			if listenErr != nil {
				log.Printf("failed to accept connection: %v", listenErr)
				continue
			}
			go w.registerClient(conn, w.tunFile, w.ctx)
		}
	}
}

func (w *TcpTunWorker) registerClient(conn net.Conn, tunFile io.ReadWriteCloser, ctx context.Context) {
	log.Printf("connected: %s", conn.RemoteAddr())
	h := handshake.NewHandshake()
	internalIPString, handshakeErr := h.ServerSideHandshake(&network.TcpAdapter{
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

	tcpConn := conn.(*net.TCPConn)
	addr := tcpConn.RemoteAddr().(*net.TCPAddr)
	internalIP := net.ParseIP(internalIPString).To4()
	externalIP := addr.IP.To4()

	// Prevent IP spoofing
	_, getErr := w.sessionManager.GetByInternalIP(internalIP)
	if !errors.Is(getErr, session_management.ErrSessionNotFound) {
		log.Printf("connection closed: %s (internal internalIP %s already in use)\n", conn.RemoteAddr(), internalIPString)
		_ = conn.Close()
	}

	storedSession := session{
		conn:                conn,
		CryptographyService: cryptographyService,
		internalIP:          internalIP.To4(),
		externalIP:          externalIP.To4(),
	}

	w.sessionManager.Add(storedSession)

	w.handleClient(ctx, storedSession, tunFile)
}

func (w *TcpTunWorker) handleClient(ctx context.Context, session session, tunFile io.ReadWriteCloser) {
	defer func() {
		w.sessionManager.Delete(session)
		_ = session.conn.Close()
		log.Printf("disconnected: %s", session.conn.RemoteAddr())
	}()

	buf := make([]byte, network.MaxPacketLengthBytes)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Read the length of the encrypted packet (4 bytes)
			_, err := io.ReadFull(session.conn, buf[:4])
			if err != nil {
				if err != io.EOF {
					log.Printf("failed to read from client: %v", err)
				}
				return
			}

			// Retrieve the session for this client
			sessionValue, getErr := w.sessionManager.GetByInternalIP(session.InternalIP())
			if getErr != nil {
				log.Printf("failed to load session for IP %s", sessionValue.InternalIP())
				continue
			}

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buf[:4])
			if length < 4 || length > network.MaxPacketLengthBytes {
				log.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(session.conn, buf[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
				continue
			}

			decrypted, decryptionErr := sessionValue.CryptographyService.Decrypt(buf[:length])
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
