package udp_chacha20

import (
	"context"
	"errors"
	"log"
	"net"
	"time"
	"tungo/crypto/chacha20"
	"tungo/network/ip"
)

type chacha20UdpWorker struct {
	router  *UDPRouter
	conn    *net.UDPConn
	session chacha20.UdpSession
}

func newChacha20UdpWorker(router *UDPRouter, conn *net.UDPConn, session chacha20.UdpSession) *chacha20UdpWorker {
	return &chacha20UdpWorker{
		router:  router,
		conn:    conn,
		session: session,
	}
}

func (w *chacha20UdpWorker) HandleTun(ctx context.Context, triggerReconnect context.CancelFunc) error {
	buf := make([]byte, ip.MaxPacketLengthBytes+12)
	udpReader := chacha20.NewUdpReader(w.router.tun)
	_ = w.conn.SetWriteBuffer(len(buf))

	// Main loop to read from TUN and send data
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			_, readErr := udpReader.Read(buf)
			if readErr != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", readErr)
				triggerReconnect()
			}

			encryptedPacket, err := w.session.Encrypt(buf)
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

func (w *chacha20UdpWorker) HandleConn(ctx context.Context, connCancel context.CancelFunc) error {
	dataBuf := make([]byte, ip.MaxPacketLengthBytes+12)
	oobBuf := make([]byte, 1024)

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

			decrypted, decryptionErr := w.session.Decrypt(dataBuf[:n])
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
