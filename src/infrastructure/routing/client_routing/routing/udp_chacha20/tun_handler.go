package udp_chacha20

import (
	"context"
	"fmt"
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
	buf := make([]byte, network.MaxPacketLengthBytes+12)

	// Main loop to read from TUN and send data
	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			n, readErr := w.reader.Read(buf)
			if readErr != nil {
				if w.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not read a packet from TUN: %v", readErr)
			}

			encryptedPacket, encryptErr := w.cryptographyService.Encrypt(buf[:n])
			if encryptErr != nil {
				if w.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not encrypt packet: %v", encryptErr)
			}

			_, writeErr := w.writer.Write(encryptedPacket)
			if writeErr != nil {
				if w.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not write packet to adapter: %v", writeErr)
			}
		}
	}
}
