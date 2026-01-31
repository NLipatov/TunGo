package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/controlplane"

	"golang.org/x/crypto/chacha20poly1305"
)

type TunHandler struct {
	ctx                 context.Context
	reader              io.Reader // abstraction over TUN device
	egress              connection.Egress
	rekeyController     *rekey.StateMachine
	controlPacketBuffer [128]byte
	rekeyInit           *controlplane.RekeyInitScheduler
}

func NewTunHandler(ctx context.Context,
	reader io.Reader,
	egress connection.Egress,
	rekeyController *rekey.StateMachine,
) tun.Handler {
	now := time.Now().UTC()
	return &TunHandler{
		ctx:             ctx,
		reader:          reader,
		egress:          egress,
		rekeyController: rekeyController,
		rekeyInit:       controlplane.NewRekeyInitScheduler(&handshake.DefaultCrypto{}, settings.DefaultRekeyInterval, now),
	}
}

// HandleTun reads packets from the TUN interface,
// reserves space for AEAD overhead, encrypts them, and forwards them to the correct session.
//
// Buffer layout (total size = MTU + NonceSize + TagSize):
//
//	[ 0 ........ 11 ][ 12 ........ 1511 ][ 1512 ........ 1527 ]
//	|   Nonce    |      Payload (<= MTU) |       Tag (16B)    |
//
// Example with MTU = 1500, settings.UDPChacha20Overhead = 28:
// - buffer length = 1500 + 28 = 1528
//
// Step 1 – read plaintext from TUN:
// - reader.Read writes at most MTU bytes into buffer[12:1512].
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
	var buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte

	// Main loop to read from TUN and send data
	for {
		select {
		case <-w.ctx.Done():
			return nil
		default:
			n, err := w.reader.Read(buffer[chacha20poly1305.NonceSize : settings.DefaultEthernetMTU+chacha20poly1305.NonceSize])
			if n > 0 {
				// Encrypt expects header+payload (12+n)
				if err := w.egress.SendDataIP(buffer[:chacha20poly1305.NonceSize+n]); err != nil {
					if w.ctx.Err() != nil {
						return nil
					}
					return fmt.Errorf("could not send packet to transport: %v", err)
				}
			}
			if err != nil {
				if w.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not read a packet from TUN: %v", err)
			}
			if w.rekeyInit != nil && w.rekeyController != nil {
				payloadBuf := w.controlPacketBuffer[chacha20poly1305.NonceSize:]
				servicePayload, ok, pErr := w.rekeyInit.MaybeBuildRekeyInit(time.Now().UTC(), w.rekeyController, payloadBuf)
				if pErr != nil {
					fmt.Printf("failed to prepare rekeyInit: %v", pErr)
					continue
				}
				if ok {
					totalLen := chacha20poly1305.NonceSize + len(servicePayload)
					if err := w.egress.SendControl(w.controlPacketBuffer[:totalLen]); err != nil {
						fmt.Printf("failed to send rekeyInit: %v", err)
					}
				}
			}
		}
	}
}
