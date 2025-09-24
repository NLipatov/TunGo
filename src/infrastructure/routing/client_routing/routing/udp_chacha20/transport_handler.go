package udp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"tungo/application"
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/settings"
)

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService application.CryptographyService
	servicePacket       service.PacketHandler
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService application.CryptographyService,
	servicePacket service.PacketHandler,
) application.TransportHandler {
	return &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		servicePacket:       servicePacket,
	}
}

func (t *TransportHandler) HandleTransport() error {
	buffer := make([]byte, settings.DefaultEthernetMTU+settings.UDPChacha20Overhead)

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, readErr := t.reader.Read(buffer)
			if readErr != nil {
				if errors.Is(readErr, os.ErrDeadlineExceeded) {
					continue
				}

				if t.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not read a packet from adapter: %v", readErr)
			}

			if spType, spOk := t.servicePacket.TryParseType(buffer[:n]); spOk {
				if spType == service.SessionReset {
					return fmt.Errorf("server requested cryptographyService reset")
				}
			}

			decrypted, decryptionErr := t.cryptographyService.Decrypt(buffer[:n])
			if decryptionErr != nil {
				if t.ctx.Err() != nil {
					return nil
				}

				// Duplicate nonce detected – this may indicate a network retransmission or a replay attack.
				// In either case, skip this packet.
				if errors.Is(decryptionErr, chacha20.ErrNonUniqueNonce) {
					continue
				}
				return fmt.Errorf("failed to decrypt data: %s", decryptionErr)
			}

			_, writeErr := t.writer.Write(decrypted)
			if writeErr != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("failed to write to TUN: %s", writeErr)
			}
		}
	}
}
