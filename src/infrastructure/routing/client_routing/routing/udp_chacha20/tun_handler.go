package udp_chacha20

import (
	"context"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"tungo/application"
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

// HandleTun reads packages from TUN-device, encrypts them and writes encrypted packages to a transport
func (w *TunHandler) HandleTun() error {
	// +12 nonce +16 AEAD tag headroom
	buffer := make([]byte, network.MaxPacketLengthBytes+chacha20poly1305.NonceSize+chacha20poly1305.Overhead)

	// Main loop to read from TUN and send data
	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			n, err := w.reader.Read(buffer[chacha20poly1305.NonceSize:])
			if n > 0 {
				// Encrypt expects header+payload (12+n)
				enc, encErr := w.cryptographyService.Encrypt(buffer[:chacha20poly1305.NonceSize+n])
				if encErr != nil {
					if w.ctx.Err() != nil {
						return nil
					}
					return fmt.Errorf("could not encrypt packet: %v", encErr)
				}
				if _, wErr := w.writer.Write(enc); wErr != nil {
					if w.ctx.Err() != nil {
						return nil
					}
					return fmt.Errorf("could not write packet to transport: %v", wErr)
				}
			}
			if err != nil {
				if w.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not read a packet from TUN: %v", err)
			}
		}
	}
}
