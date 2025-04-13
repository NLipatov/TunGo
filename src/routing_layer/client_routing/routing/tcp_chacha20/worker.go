package tcp_chacha20

import (
	"context"
	"encoding/binary"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"log"
	"net"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
)

type TcpTunWorker struct {
	conn                net.Conn
	tun                 io.ReadWriteCloser
	cryptographyService application.CryptographyService
}

func NewTcpTunWorker(
	conn net.Conn, tun io.ReadWriteCloser, cryptographyService application.CryptographyService,
) application.TunWorker {
	return &TcpTunWorker{
		conn:                conn,
		tun:                 tun,
		cryptographyService: cryptographyService,
	}
}

func (w *TcpTunWorker) HandleTun(ctx context.Context) error {
	reader := chacha20.NewTcpReader(w.tun)
	buffer := make([]byte, network.MaxPacketLengthBytes+4+chacha20poly1305.Overhead)
	tcpEncoder := chacha20.NewDefaultTCPEncoder()

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
			n, err := reader.Read(buffer)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				return err
			}

			_, err = w.cryptographyService.Encrypt(buffer[4 : n+4])
			if err != nil {
				log.Printf("failed to encrypt packet: %v", err)
				continue
			}

			encodingErr := tcpEncoder.Encode(buffer[:n+4+chacha20poly1305.Overhead])
			if encodingErr != nil {
				log.Printf("failed to encode packet: %v", encodingErr)
				continue
			}

			_, err = w.conn.Write(buffer[:n+4+chacha20poly1305.Overhead])
			if err != nil {
				log.Printf("write to TCP failed: %s", err)
				return err
			}
		}
	}
}

func (w *TcpTunWorker) HandleTransport(ctx context.Context) error {
	buffer := make([]byte, network.MaxPacketLengthBytes+4)

	go func() {
		<-ctx.Done()
		_ = w.conn.Close()
	}()

	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			_, err := io.ReadFull(w.conn, buffer[:4])
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("read from TCP failed: %v", err)
				return err
			}

			//read packet length from 4-byte length prefix
			var length = binary.BigEndian.Uint32(buffer[:4])
			if length < 4 || length > network.MaxPacketLengthBytes {
				log.Printf("invalid packet Length: %d", length)
				continue
			}

			//read n-bytes from connection
			_, err = io.ReadFull(w.conn, buffer[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
				continue
			}

			decrypted, decryptionErr := w.cryptographyService.Decrypt(buffer[:length])
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
				continue
			}

			// Write the decrypted packet to the TUN interface
			_, err = w.tun.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
				return err
			}
		}
	}
}
