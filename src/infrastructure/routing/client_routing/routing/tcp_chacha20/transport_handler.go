package tcp_chacha20

import (
	"context"
	"encoding/binary"
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
	cryptographyService application.CryptographyService) application.TransportHandler {
	return &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
	}
}

func (t *TransportHandler) HandleTransport() error {
	buffer := make([]byte, network.MaxPacketLengthBytes+4)

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			_, err := io.ReadFull(t.reader, buffer[:4])
			if err != nil {
				if t.ctx.Err() != nil {
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
			_, err = io.ReadFull(t.reader, buffer[:length])
			if err != nil {
				log.Printf("failed to read packet from connection: %s", err)
				continue
			}

			decrypted, decryptionErr := t.cryptographyService.Decrypt(buffer[:length])
			if decryptionErr != nil {
				log.Printf("failed to decrypt data: %s", decryptionErr)
				return decryptionErr
			}

			// Write the decrypted packet to the TUN interface
			_, err = t.writer.Write(decrypted)
			if err != nil {
				log.Printf("failed to write to TUN: %v", err)
				return err
			}
		}
	}
}
