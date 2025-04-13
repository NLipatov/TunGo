package udp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
)

type UdpWorker struct {
	conn                net.UDPConn
	tun                 io.ReadWriteCloser
	cryptographyService application.CryptographyService
}

func NewUdpWorker(
	conn net.Conn, tun io.ReadWriteCloser, cryptographyService application.CryptographyService,
) *UdpWorker {
	return &UdpWorker{
		conn:                *conn.(*net.UDPConn),
		tun:                 tun,
		cryptographyService: cryptographyService,
	}
}

func (w *UdpWorker) HandleTun(ctx context.Context) error {
	buf := make([]byte, network.MaxPacketLengthBytes+12)
	udpReader := chacha20.NewUdpReader(w.tun)
	_ = w.conn.SetWriteBuffer(len(buf))

	// Main loop to read from TUN and send data
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			n, readErr := udpReader.Read(buf)
			if readErr != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not read a packet from TUN: %v", readErr)
			}

			encryptedPacket, EncryptErr := w.cryptographyService.Encrypt(buf[:n])
			if EncryptErr != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not encrypt packet: %v", EncryptErr)
			}

			_ = w.conn.SetWriteDeadline(time.Now().Add(time.Second * 2))
			_, writeErr := w.conn.Write(encryptedPacket)
			if writeErr != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not write packet to conn: %v", writeErr)
			}
		}
	}
}

func (w *UdpWorker) HandleTransport(ctx context.Context) error {
	dataBuf := make([]byte, network.MaxPacketLengthBytes+12)
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
				return fmt.Errorf("could not read a packet from conn: %v", readErr)
			}

			if n == 1 {
				if network.SignalIs(dataBuf[:n][0], network.SessionReset) {
					return fmt.Errorf("server requested cryptographyService reset")
				}
			}

			decrypted, decryptionErr := w.cryptographyService.Decrypt(dataBuf[:n])
			if decryptionErr != nil {
				if ctx.Err() != nil {
					return nil
				}

				// Duplicate nonce detected – this may indicate a network retransmission or a replay attack.
				// In either case, skip this packet.
				if errors.Is(decryptionErr, chacha20.ErrNonUniqueNonce) {
					continue
				}
				return fmt.Errorf("failed to decrypt data: %s", decryptionErr)
			}

			_, writeErr := w.tun.Write(decrypted)
			if writeErr != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("failed to write to TUN: %s", writeErr)
			}
		}
	}
}
