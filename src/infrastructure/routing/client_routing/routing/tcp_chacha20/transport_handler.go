package tcp_chacha20

import (
	"context"
	"io"
	"log"
	"tungo/application"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService application.CryptographyService
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService application.CryptographyService,
) application.TransportHandler {
	return &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
	}
}

func (t *TransportHandler) HandleTransport() error {
	buf := make([]byte, settings.MTU+settings.TCPChacha20Overhead)
	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, err := t.reader.Read(buf)
			if err != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				log.Printf("read from TCP failed: %v", err)
				return err
			}

			if n < chacha20poly1305.Overhead || n > settings.MTU+settings.TCPChacha20Overhead {
				log.Printf("invalid ciphertext length: %d", n)
				continue
			}

			pt, err := t.cryptographyService.Decrypt(buf[:n])
			if err != nil {
				log.Printf("failed to decrypt data: %s", err)
				return err
			}
			if _, err = t.writer.Write(pt); err != nil {
				log.Printf("failed to write to TUN: %v", err)
				return err
			}
		}
	}
}
