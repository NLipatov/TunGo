package udp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
	"tungo/infrastructure/network/ip"
)

type chacha20UdpWorker struct {
	router              *UDPRouter
	conn                *net.UDPConn
	cryptographyService application.CryptographyService
}

func newChacha20UdpWorker(router *UDPRouter, conn *net.UDPConn, cryptographyService application.CryptographyService) *chacha20UdpWorker {
	return &chacha20UdpWorker{
		router:              router,
		conn:                conn,
		cryptographyService: cryptographyService,
	}
}

func (w *chacha20UdpWorker) HandleTun(ctx context.Context, cancelFunc context.CancelFunc) error {
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

				cancelFunc()
				return fmt.Errorf("could not read a packet from TUN: %v", readErr)
			}

			encryptedPacket, EncryptErr := w.cryptographyService.Encrypt(buf)
			if EncryptErr != nil {
				if ctx.Err() != nil {
					return nil
				}

				cancelFunc()
				return fmt.Errorf("could not encrypt packet: %v", EncryptErr)
			}

			_ = w.conn.SetWriteDeadline(time.Now().Add(time.Second * 2))
			_, writeErr := w.conn.Write(encryptedPacket)
			if writeErr != nil {
				if ctx.Err() != nil {
					return nil
				}

				cancelFunc()
				return fmt.Errorf("could not write packet to conn: %v", writeErr)
			}
		}
	}
}

func (w *chacha20UdpWorker) HandleConn(ctx context.Context, cancelFunc context.CancelFunc) error {
	dataBuf := make([]byte, ip.MaxPacketLengthBytes+12)
	oobBuf := make([]byte, 1024)
	_ = w.conn.SetReadBuffer(len(dataBuf))

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

				cancelFunc()
				return fmt.Errorf("could not read a packet from conn: %v", readErr)
			}

			if n == 1 {
				if network.SignalIs(dataBuf[:n][0], network.SessionReset) {
					cancelFunc()
					return fmt.Errorf("server requested cryptographyService reset")
				}
			}

			decrypted, decryptionErr := w.cryptographyService.Decrypt(dataBuf[:n])
			if decryptionErr != nil {
				if ctx.Err() != nil {
					return nil
				}

				if errors.Is(decryptionErr, chacha20.ErrNonUniqueNonce) {
					cancelFunc()
					return fmt.Errorf("reconnecting on critical decryption err: %s", decryptionErr)
				}

				cancelFunc()
				return fmt.Errorf("failed to decrypt data: %s", decryptionErr)
			}

			_, writeErr := w.router.tun.Write(decrypted)
			if writeErr != nil {
				if ctx.Err() != nil {
					return nil
				}

				cancelFunc()
				return fmt.Errorf("failed to write to TUN: %s", writeErr)
			}
		}
	}
}
