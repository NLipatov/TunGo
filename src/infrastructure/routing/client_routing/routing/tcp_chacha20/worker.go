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
	ctx                 context.Context
	conn                net.Conn
	tun                 io.ReadWriteCloser
	cryptographyService application.CryptographyService
}

func NewTcpTunWorker(
	ctx context.Context, conn net.Conn, tun io.ReadWriteCloser, cryptographyService application.CryptographyService,
) application.TunWorker {
	return &TcpTunWorker{
		ctx:                 ctx,
		conn:                conn,
		tun:                 tun,
		cryptographyService: cryptographyService,
	}
}

func (w *TcpTunWorker) HandleTun() error {
	reader := chacha20.NewTcpReader(w.tun)
	buffer := make([]byte, network.MaxPacketLengthBytes+4+chacha20poly1305.Overhead)
	tcpEncoder := chacha20.NewDefaultTCPEncoder()

	//passes anything from tun to chan
	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			n, err := reader.Read(buffer)
			if err != nil {
				if w.ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				return err
			}

			_, encryptErr := w.cryptographyService.Encrypt(buffer[4 : n+4])
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %v", encryptErr)
				return encryptErr
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

func (w *TcpTunWorker) HandleTransport() error {
	buffer := make([]byte, network.MaxPacketLengthBytes+4)

	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			_, err := io.ReadFull(w.conn, buffer[:4])
			if err != nil {
				if w.ctx.Err() != nil {
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
				return decryptionErr
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
