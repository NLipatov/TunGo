package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"log"
	"net"
	"tungo/crypto"
	"tungo/crypto/chacha20"
	"tungo/network"
	"tungo/settings/client"
)

type tcpTunWorker struct {
	router  *TCPRouter
	conn    net.Conn
	session crypto.Session
	encoder chacha20.TCPEncoder
	err     error
}

func newTcpTunWorker() *tcpTunWorker {
	return &tcpTunWorker{}
}

func (w *tcpTunWorker) UseRouter(router *TCPRouter) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.router = router
	return w
}

func (w *tcpTunWorker) UseSession(session crypto.Session) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.session = session

	return w
}

func (w *tcpTunWorker) UseConn(conn net.Conn) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.conn = conn

	return w
}

func (w *tcpTunWorker) UseEncoder(encoder *chacha20.DefaultTCPEncoder) *tcpTunWorker {
	if w.err != nil {
		return w
	}

	w.encoder = encoder

	return w
}

func (w *tcpTunWorker) Build() (*tcpTunWorker, error) {
	if w.err != nil {
		return nil, w.err
	}

	if w.router == nil {
		return nil, fmt.Errorf("router required but not provided")
	}

	if w.session == nil {
		return nil, fmt.Errorf("session required but not provided")
	}

	if w.conn == nil {
		return nil, fmt.Errorf("connection required but not provided")
	}

	if w.encoder == nil {
		return nil, fmt.Errorf("encoder required but not provided")
	}

	return w, nil
}

func (w *tcpTunWorker) HandlePacketsFromTun(ctx context.Context, triggerReconnect context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	buf := make([]byte, network.IPPacketMaxSizeBytes+4+chacha20poly1305.Overhead)

	go func() {
		<-ctx.Done()
		_ = w.conn.Close()
	}()

	//passes anything from tun to chan
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			n, err := w.router.tun.Read(buf[4:])
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				triggerReconnect()
			}

			_, err = w.session.Encrypt(buf[4 : n+4])
			if err != nil {
				log.Printf("failed to encrypt packet: %v", err)
				continue
			}

			_, err = w.encoder.Encode(buf[:n+4+chacha20poly1305.Overhead])
			if err != nil {
				log.Printf("packet encoding failed: %s", err)
				continue
			}

			_, err = w.conn.Write(buf[:n+4+chacha20poly1305.Overhead])
			if err != nil {
				log.Printf("write to TCP failed: %s", err)
				triggerReconnect()
			}
		}
	}
}

func (w *tcpTunWorker) HandlePacketsFromConn(ctx context.Context, connCancel context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	buf := make([]byte, network.IPPacketMaxSizeBytes)

	go func() {
		<-ctx.Done()
		_ = w.conn.Close()
	}()

	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			_, err := io.ReadFull(w.conn, buf[:4])
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("read from TCP failed: %v", err)
				connCancel()
			}

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buf[:4])
			if length < 4 || length > network.IPPacketMaxSizeBytes {
				log.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(w.conn, buf[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
				continue
			}

			packet, err := w.encoder.Decode(buf[:length])
			if err != nil {
				log.Println(err)
			}

			decrypted, decryptionErr := w.session.Decrypt(packet.Payload)
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = w.router.tun.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
				return err
			}
		}
	}
}

func getConnWriteBufferSize() int32 {
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Println("failed to read connection buffer size from client configuration. Using fallback value: 125 000")
		return 125_000
	}

	return conf.TCPWriteChannelBufferSize
}
