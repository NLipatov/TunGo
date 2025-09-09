package udp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"tungo/application"
	appip "tungo/application/network/ip"
	"tungo/infrastructure/cryptography/chacha20"
	ipimpl "tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/signaling"
	"tungo/infrastructure/settings"
)

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService application.CryptographyService
	ipParser            appip.HeaderParser
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
		ipParser:            ipimpl.NewHeaderParser(),
	}
}

func (t *TransportHandler) HandleTransport() error {
	buffer := make([]byte, settings.MTU+settings.UDPChacha20Overhead)

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

			if n == 1 && signaling.SignalIs(buffer[0], signaling.SessionReset) {
				return fmt.Errorf("server requested cryptographyService reset")
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

			// Inspect destination address for service frames.
			if addr, err := t.ipParser.DestinationAddress(decrypted); err == nil && addr == application.ServiceIP {
				if len(decrypted) > 20 {
					typ := decrypted[20]
					if typ == application.MTUProbeType {
						if conn, ok := t.reader.(application.ConnectionAdapter); ok {
							ackPkt := application.BuildMTUPacket(application.MTUAckType, len(decrypted))
							if enc, err := t.cryptographyService.Encrypt(ackPkt); err == nil {
								_, _ = conn.Write(enc)
							}
						}
					}
					// Drop service frames (probe or ack) from reaching TUN.
					continue
				}
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
