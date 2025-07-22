package tcp_chacha20

import (
	"context"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"log"
	"tungo/application"
	"tungo/infrastructure/network"
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
		ctx: ctx, reader: reader, writer: writer, cryptographyService: cryptographyService,
	}
}

func (t *TransportHandler) HandleTransport() error {
	buffer := make([]byte, network.MaxPacketLengthBytes+chacha20poly1305.Overhead)
	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
		}
		n, err := t.reader.Read(buffer)
		if err != nil {
			log.Printf("read from TCP failed: %v", err)
			return err
		}
		decrypted, decryptionErr := t.cryptographyService.Decrypt(buffer[:n])
		if decryptionErr != nil {
			log.Printf("failed to decrypt data: %s", decryptionErr)
			return decryptionErr
		}
		_, err = t.writer.Write(decrypted)
		if err != nil {
			log.Printf("failed to write to TUN: %v", err)
			return err
		}
	}
}
