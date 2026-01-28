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
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/routing/udp"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/curve25519"
)

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService connection.Crypto
	rekeyController     *rekey.StateMachine
	servicePacket       service.PacketHandler
	handshakeCrypto     handshake.Crypto
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	rekeyController *rekey.StateMachine,
	servicePacket service.PacketHandler,
) transport.Handler {
	return &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekeyController:     rekeyController,
		servicePacket:       servicePacket,
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

			if spType, spOk := t.servicePacket.TryParseType(buffer[:n]); spOk {
				t.rekeyController.MaybeAbortPending(time.Now())
				if spType == service.SessionReset {
					return fmt.Errorf("server requested cryptographyService reset")
				}
			}

			if n < 2 {
				fmt.Printf("packet too short for epoch: %d bytes\n", n)
				continue
			}
			epoch := binary.BigEndian.Uint16(buffer[:2])
			t.rekeyController.MaybeAbortPending(time.Now())
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

			if spType, spOk := t.servicePacket.TryParseType(decrypted); spOk {
				switch spType {
				case service.RekeyAck:
					if t.rekeyController.LastRekeyEpoch >= 65000 {
						fmt.Println("rekey ack: epoch exhausted, requesting session reset")
						return fmt.Errorf("epoch exhausted; reconnect required")
					}
					if len(decrypted) < service.RekeyPacketLen {
						fmt.Printf("rekey ack too short: %d bytes\n", len(decrypted))
						continue
					}
					priv, ok := t.rekeyController.PendingRekeyPrivateKey()
					if !ok {
						fmt.Println("rekey ack: no pending client private key")
						continue
					}
					serverPub := decrypted[3 : 3+service.RekeyPublicKeyLen]
					shared, err := curve25519.X25519(priv[:], serverPub)
					if err != nil {
						fmt.Printf("rekey ack: failed to compute shared secret: %v\n", err)
						continue
					}
					currentC2S := t.rekeyController.CurrentClientToServerKey()
					currentS2C := t.rekeyController.CurrentServerToClientKey()
					newC2S, err := t.handshakeCrypto.DeriveKey(shared, currentC2S, []byte("tungo-rekey-c2s"))
					if err != nil {
						fmt.Printf("rekey ack: derive key failed: %v\n", err)
						continue
					}
					newS2C, err := t.handshakeCrypto.DeriveKey(shared, currentS2C, []byte("tungo-rekey-s2c"))
					if err != nil {
						fmt.Printf("rekey ack: derive key failed: %v\n", err)
						continue
					}
					epoch, err := t.rekeyController.RekeyAndApply(newC2S, newS2C)
					if err != nil {
						fmt.Printf("rekey ack: install/apply failed: %v\n", err)
						continue
					}
					// Initiator proactively switches send to drive peer confirmation.
					t.rekeyController.PromoteSendEpoch(epoch)
					t.rekeyController.ClearPendingRekeyPrivateKey()
				case service.SessionReset:
					return fmt.Errorf("server requested cryptographyService reset")
				default:
					// ignore unknown service packets
				}
				continue
			}

			// Only confirm epoch on actual data packets (non-service, non-multicast)
			if !udp.IsMulticastPacket(decrypted) {
				t.rekeyController.PromoteSendEpoch(epoch)
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
