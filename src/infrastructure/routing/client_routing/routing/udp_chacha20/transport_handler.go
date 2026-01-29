package udp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/routing/controlplane"
	"tungo/infrastructure/settings"
)

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService connection.Crypto
	rekeyController     *rekey.StateMachine
	handshakeCrypto     handshake.Crypto
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	rekeyController *rekey.StateMachine,
) transport.Handler {
	return &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekeyController:     rekeyController,
		handshakeCrypto:     &handshake.DefaultCrypto{},
	}
}

func (t *TransportHandler) HandleTransport() error {
	var buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, readErr := t.reader.Read(buffer[:])
			if readErr != nil {
				if errors.Is(readErr, os.ErrDeadlineExceeded) {
					continue
				}
				if t.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not read a packet from adapter: %v", readErr)
			}
			if spType, spOk := service_packet.TryParseHeader(buffer[:n]); spOk {
				t.rekeyController.AbortPendingIfExpired(time.Now())
				if spType == service_packet.SessionReset {
					return fmt.Errorf("server requested cryptographyService reset")
				}
			}
			if n < 2 {
				continue
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
			// Data was successfully decrypted with epoch.
			// Epoch can now be used to encrypt. Allow to encrypt with this epoch by promoting.
			t.rekeyController.ActivateSendEpoch(binary.BigEndian.Uint16(buffer[:2]))
			t.rekeyController.AbortPendingIfExpired(time.Now())

			if spType, spOk := service_packet.TryParseHeader(decrypted); spOk {
				switch spType {
				case service_packet.RekeyAck:
					if t.rekeyController.LastRekeyEpoch >= 65000 {
						fmt.Println("rekey ack: epoch exhausted, requesting session reset")
						return fmt.Errorf("epoch exhausted; reconnect required")
					}
					if _, err := controlplane.ClientHandleRekeyAck(t.handshakeCrypto, t.rekeyController, decrypted); err != nil {
						fmt.Printf("rekey ack: install/apply failed: %v\n", err)
					}
				case service_packet.SessionReset:
					return fmt.Errorf("server requested cryptographyService reset")
				default:
					// ignore unknown service_packet packets
				}
				continue
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
