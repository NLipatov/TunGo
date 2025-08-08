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
	encoder             chacha20.TCPEncoder
}

func NewTunHandler(ctx context.Context,
	encoder chacha20.TCPEncoder,
	reader io.Reader,
	writer io.Writer,
	cryptographyService application.CryptographyService) application.TunHandler {
	return &TunHandler{
		ctx:                 ctx,
		encoder:             encoder,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
	}
}

func (t *TunHandler) HandleTun() error {
	buffer := make([]byte, 4+network.MaxPacketLengthBytes+chacha20poly1305.Overhead)

	//passes anything from tun to chan
	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := t.reader.Read(buffer[4:])
			if err != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				log.Printf("failed to read from TUN: %v", err)
				return err
			}

			ct, encryptErr := t.cryptographyService.Encrypt(buffer[4 : n+4])
			if encryptErr != nil {
				log.Printf("failed to encrypt packet: %v", encryptErr)
				return encryptErr
			}

			frame := buffer[:4+len(ct)]

			encodingErr := t.encoder.Encode(frame)
			if encodingErr != nil {
				log.Printf("failed to encode packet: %v", encodingErr)
				continue
			}

			_, err = t.writer.Write(frame)
			if err != nil {
				log.Printf("write to TCP failed: %s", err)
				return err
			}
		}
	}
}
