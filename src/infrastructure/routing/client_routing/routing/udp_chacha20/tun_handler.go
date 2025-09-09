package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"tungo/application"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
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

// HandleTun reads packets from the TUN interface,
// reserves space for AEAD overhead, encrypts them, and forwards them to the correct session.
//
// Buffer layout (total size = SafeMTU + NonceSize + TagSize):
//
//	[ 0 ........ 11 ][ 12 ........ 1511 ][ 1512 ........ 1527 ]
//	|   Nonce    |      Payload (<= SafeMTU) |       Tag (16B)    |
//
// Example with settings.SafeMTU = 1500, settings.UDPChacha20Overhead = 28:
// - buffer length = 1500 + 28 = 1528
//
// Step 1 – read plaintext from TUN:
// - reader.Read writes at most SafeMTU bytes into buffer[12:1512].
// - the first 12 bytes (buffer[0:12]) are reserved for the nonce
// - the last 16 bytes (buffer[1512:1528]) are reserved for the Poly1305 tag
//
// Step 2 – encrypt plaintext in place:
//   - encryption operates on buffer[0 : 12+n] (nonce + payload)
//   - ciphertext and authentication tag are written back in place
//   - no additional allocations are required since both the prefix
//     (nonce) and the suffix (tag) are already reserved in the buffer.
func (w *TunHandler) HandleTun() error {
	// +12 nonce +16 AEAD tag headroom
	buffer := make([]byte, settings.DefaultEthernetMTU+settings.UDPChacha20Overhead)

	// Main loop to read from TUN and send data
	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			n, err := w.reader.Read(buffer[chacha20poly1305.NonceSize : settings.DefaultEthernetMTU+chacha20poly1305.NonceSize])
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
