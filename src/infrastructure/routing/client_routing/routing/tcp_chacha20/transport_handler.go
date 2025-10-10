package tcp_chacha20

import (
	"context"
	"io"
	"log"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService connection.Crypto
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
) transport.Handler {
	return &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
	}
}

func (t *TransportHandler) HandleTransport() error {
	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, readErr := t.reader.Read(buffer[:])
			if readErr != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				log.Printf("read from TCP failed: %v", readErr)
				return readErr
			}

			if n < chacha20poly1305.Overhead || n > settings.DefaultEthernetMTU+settings.TCPChacha20Overhead {
				log.Printf("invalid ciphertext length: %d", n)
				continue
			}

			payload, payloadErr := t.cryptographyService.Decrypt(buffer[:n])
			if payloadErr != nil {
				log.Printf("failed to decrypt data: %s", payloadErr)
				return payloadErr
			}
			if _, writeErr := t.writer.Write(payload); writeErr != nil {
				log.Printf("failed to write to TUN: %v", writeErr)
				return writeErr
			}
		}
	}
}
