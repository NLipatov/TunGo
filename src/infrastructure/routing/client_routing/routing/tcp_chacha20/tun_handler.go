package tcp_chacha20

import (
	"context"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"log"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
)

type TunHandler struct {
	ctx                 context.Context
	reader              io.Reader // abstraction over TUN device
	writer              io.Writer // abstraction over transport
	cryptographyService application.CryptographyService
}

func NewTunHandler(ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService application.CryptographyService) application.TunHandler {
	return &TunHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
	}
}

func (t *TunHandler) HandleTun() error {
	reader := chacha20.NewTcpReader(t.reader)
	buffer := make([]byte, network.MaxPacketLengthBytes+4+chacha20poly1305.Overhead)
	tcpEncoder := chacha20.NewDefaultTCPEncoder()

	//passes anything from tun to chan
	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := reader.Read(buffer)
			if err != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				return err
			}

			_, encryptErr := t.cryptographyService.Encrypt(buffer[4 : n+4])
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %v", encryptErr)
				return encryptErr
			}

			encodingErr := tcpEncoder.Encode(buffer[:n+4+chacha20poly1305.Overhead])
			if encodingErr != nil {
				log.Printf("failed to encode packet: %v", encodingErr)
				continue
			}

			_, err = t.writer.Write(buffer[:n+4+chacha20poly1305.Overhead])
			if err != nil {
				log.Printf("write to TCP failed: %s", err)
				return err
			}
		}
	}
}
