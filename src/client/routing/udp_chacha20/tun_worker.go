package udp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
	"tungo/crypto/chacha20"
	"tungo/network/ip"
)

type udpTunWorker struct {
	router  *UDPRouter
	conn    *net.UDPConn
	session chacha20.UdpSession
	err     error
	encoder chacha20.UDPEncoder
}

func newUdpTunWorker() *udpTunWorker {
	return &udpTunWorker{}
}

func (w *udpTunWorker) UseRouter(router *UDPRouter) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.router = router
	return w
}

func (w *udpTunWorker) UseSession(session chacha20.UdpSession) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.session = session

	return w
}

func (w *udpTunWorker) UseConn(conn *net.UDPConn) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.conn = conn

	return w
}

func (w *udpTunWorker) UseEncoder(encoder chacha20.UDPEncoder) *udpTunWorker {
	if w.err != nil {
		return w
	}

	w.encoder = encoder

	return w
}

func (w *udpTunWorker) Build() (*udpTunWorker, error) {
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

func (w *udpTunWorker) HandlePacketsFromTun(ctx context.Context, triggerReconnect context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	buf := make([]byte, ip.MaxPacketLengthBytes+12)
	udpReader := chacha20.NewUdpReader(buf, w.router.tun)
	_ = w.conn.SetWriteBuffer(len(buf))

	// Main loop to read from TUN and send data
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			_, readErr := udpReader.Read()
			if readErr != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", readErr)
				triggerReconnect()
			}

			encryptedPacket, err := w.session.InplaceEncrypt(buf)
			if err != nil {
				log.Printf("failed to encrypt packet: %v", err)
				continue
			}

			_ = w.conn.SetWriteDeadline(time.Now().Add(time.Second * 2))
			_, err = w.conn.Write(encryptedPacket)
			if err != nil {
				log.Printf("write to UDP failed: %s", err)
				if ctx.Err() != nil {
					return nil
				}
				triggerReconnect()
			}
		}
	}
}

func (w *udpTunWorker) HandlePacketsFromConn(ctx context.Context, connCancel context.CancelFunc) error {
	workerSetupErr := w.err
	if workerSetupErr != nil {
		return workerSetupErr
	}
	dataBuf := make([]byte, ip.MaxPacketLengthBytes+12)
	oobBuf := make([]byte, 1024)
	err := w.conn.SetReadBuffer(4 * 1024 * 1024)
	if err != nil {
		log.Printf("SetReadBuffer error: %v", err)
	}
	err = w.conn.SetWriteBuffer(4 * 1024 * 1024)
	if err != nil {
		log.Printf("SetWriteBuffer error: %v", err)
	}

	go func() {
		<-ctx.Done()
		_ = w.conn.Close()
	}()

	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			n, _, _, _, readErr := w.conn.ReadMsgUDP(dataBuf, oobBuf)
			if readErr != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("read from UDP failed: %v", readErr)
				connCancel()
				return readErr
			}

			decrypted, decryptionErr := w.session.InplaceDecrypt(dataBuf[:n])
			if decryptionErr != nil {
				if errors.Is(decryptionErr, chacha20.ErrNonUniqueNonce) {
					log.Printf("reconnecting on critical decryption err: %s", decryptionErr)
					connCancel()
					return nil
				}
				log.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			_, writeErr := w.router.tun.Write(decrypted)
			if writeErr != nil {
				log.Printf("failed to write to TUN: %v", writeErr)
				return writeErr
			}
		}
	}
}
