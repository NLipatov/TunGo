package udp_chacha20

import (
	"context"
	"fmt"
	"io"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/rekey"
	"tungo/application/network/routing/tun"
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

type TunHandler struct {
	ctx                 context.Context
	reader              io.Reader // abstraction over TUN device
	writer              io.Writer // abstraction over transport
	cryptographyService connection.Crypto
	rekeyController     *rekey.Controller
	servicePacket       service.PacketHandler
	controlPacketBuffer [128]byte
	rotateAt            time.Time
	handshakeCrypto     handshake.Crypto
}

func NewTunHandler(ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	rekeyController *rekey.Controller,
	servicePacket service.PacketHandler,
) tun.Handler {
	return &TunHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekeyController:     rekeyController,
		servicePacket:       servicePacket,
		rotateAt:            time.Now().UTC().Add(10 * time.Second),
		handshakeCrypto:     &handshake.DefaultCrypto{},
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
			if time.Now().UTC().After(w.rotateAt) {
				publicKey, privateKey, keyErr := w.handshakeCrypto.GenerateX25519KeyPair()
				if keyErr != nil {
					fmt.Printf("failed to generate rekey key pair: %v", keyErr)
					w.rotateAt = time.Now().UTC().Add(10 * time.Second)
					continue
				}
				// Controller must always be present for UDP; panic on misconfiguration.
				w.rekeyController.SetPendingRekeyPrivateKey(privateKey)

				payloadBuf := w.controlPacketBuffer[chacha20poly1305.NonceSize:]
				if len(publicKey) != service.RekeyPublicKeyLen {
					fmt.Println("unexpected rekey public key length")
					w.rotateAt = time.Now().UTC().Add(10 * time.Second)
					continue
				}
				copy(payloadBuf[3:], publicKey)

				servicePayload, err := w.servicePacket.EncodeV1(service.RekeyInit, payloadBuf)
				if err != nil {
					fmt.Println("failed to encode rekeyInit packet")
				} else {
					totalLen := chacha20poly1305.NonceSize + len(servicePayload)
					enc, encErr := w.cryptographyService.Encrypt(w.controlPacketBuffer[:totalLen])
					if encErr != nil {
						fmt.Printf("failed to encrypt packet: %v", encErr)
					} else {
						_, _ = w.writer.Write(enc)
					}
				}
				w.rotateAt = time.Now().UTC().Add(10 * time.Second)
			}
		}
	}
}
