package tun_tcp_chacha20

import (
	"context"
	"encoding/binary"
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
	"tungo/Domain/settings/server"
	"tungo/Infrastructure/network/packets"
)

type TCPRouter struct {
	settings            settings.ConnectionSettings
	tun                 boundary.TunAdapter
	localIpMap          *sync.Map
	localIpToSessionMap *sync.Map
	ctx                 context.Context
	err                 error
}

func (r *TCPRouter) TunToTCP() {
	buf := make([]byte, Domain.IPPacketMaxSizeBytes)
	connWriteChan := make(chan clientData, r.getConnWriteBufferSize())

	//starts a goroutine that writes whatever comes from chan to TCP
	go r.processConnWriteChan(connWriteChan)

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

			header, err := packets.Parse(data)
			if err != nil {
				log.Printf("failed to parse a IPv4 header")
				continue
			}
			destinationIP := header.GetDestinationIP().String()
			v, ok := r.localIpMap.Load(destinationIP)
			if ok {
				conn := v.(net.Conn)
				sessionValue, sessionExists := r.localIpToSessionMap.Load(destinationIP)
				if !sessionExists {
					log.Printf("failed to load session")
					continue
				}
				session := sessionValue.(*chacha20.Session)
				encryptedPacket, _, encryptErr := session.Encrypt(data)
				if encryptErr != nil {
					log.Printf("failder to encrypt a package: %s", encryptErr)
					continue
				}

				packet, packetEncodeErr := (&chacha20.TCPEncoder{}).Encode(encryptedPacket)
				if packetEncodeErr != nil {
					log.Printf("packet encoding failed: %s", packetEncodeErr)
				}

				connWriteChan <- clientData{
					Conn:  conn,
					ExtIP: destinationIP,
					Data:  packet.Payload,
				}
			}
		}
	}
}

func (r *TCPRouter) TCPToTun() {
	listener, err := net.Listen("tcp", r.settings.Port)
	if err != nil {
		log.Printf("failed to listen on port %s: %v", r.settings.Port, err)
	}
	defer func() {
		_ = listener.Close()
	}()
	log.Printf("server listening on port %s (TCP)", r.settings.Port)

	//using this goroutine to 'unblock' Listener.Accept blocking-call
	go func() {
		<-r.ctx.Done() //blocks till ctx.Done signal comes in
		_ = listener.Close()
		return
	}()

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			conn, listenErr := listener.Accept()
			if r.ctx.Err() != nil {
				log.Printf("exiting Accept loop: %s", r.ctx.Err())
				return
			}
			if listenErr != nil {
				log.Printf("failed to accept connection: %v", listenErr)
				continue
			}
			go r.registerClient(conn)
		}
	}
}

func (r *TCPRouter) processConnWriteChan(connWriteChan chan clientData) {
	for {
		select {
		case <-r.ctx.Done(): // Stop-signal
			close(connWriteChan)
			return
		case data := <-connWriteChan:
			_, connWriteErr := data.Conn.Write(data.Data)
			if connWriteErr != nil {
				log.Printf("failed to write to TCP: %v", connWriteErr)
				r.localIpMap.Delete(data.ExtIP)
				r.localIpToSessionMap.Delete(data.ExtIP)
			}
		}
	}
}

func (r *TCPRouter) getConnWriteBufferSize() int32 {
	conf, err := (&server.Conf{}).Read()
	if err != nil {
		log.Println("failed to read connection buffer size from client configuration. Using fallback value: 1000")
		return 1000
	}

	return conf.TCPWriteChannelBufferSize
}

func (r *TCPRouter) registerClient(conn net.Conn) {
	log.Printf("connected: %s", conn.RemoteAddr())

	serverSession, internalIpAddr, err := handshake.OnClientConnected(conn)
	if err != nil {
		_ = conn.Close()
		log.Printf("conn closed: %s (regfail: %s)\n", conn.RemoteAddr(), err)
		return
	}
	log.Printf("registered: %s", conn.RemoteAddr())

	// Prevent IP spoofing
	_, ipCollision := r.localIpMap.Load(*internalIpAddr)
	if ipCollision {
		log.Printf("conn closed: %s (internal ip %s already in use)\n", conn.RemoteAddr(), *internalIpAddr)
		_ = conn.Close()
	}

	r.localIpMap.Store(*internalIpAddr, conn)
	r.localIpToSessionMap.Store(*internalIpAddr, serverSession)

	r.handleClient(conn, internalIpAddr)
}

func (r *TCPRouter) handleClient(conn net.Conn, extIpAddr *string) {
	defer func() {
		r.localIpMap.Delete(*extIpAddr)
		r.localIpToSessionMap.Delete(*extIpAddr)
		_ = conn.Close()
		log.Printf("disconnected: %s", conn.RemoteAddr())
	}()

	buf := make([]byte, Domain.IPPacketMaxSizeBytes)
	for {
		select {
		case <-r.ctx.Done():
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
			sessionValue, sessionExists := r.localIpToSessionMap.Load(*extIpAddr)
			if !sessionExists {
				log.Printf("failed to load session for IP %s", *extIpAddr)
				continue
			}

			session := sessionValue.(*chacha20.Session)

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buf[:4])
			if length < 4 || length > Domain.IPPacketMaxSizeBytes {
				log.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
				continue
			}

			decrypted, _, decryptionErr := session.Decrypt(buf[:length])
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = r.tun.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
				return
			}
		}
	}
}
