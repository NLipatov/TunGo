package udp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"tungo/application/network/connection"
	"tungo/application/network/rekey"
	"tungo/application/network/routing/transport"
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/routing/udp"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/curve25519"
)

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService connection.Crypto
	rekeyController     *rekey.Controller
	servicePacket       service.PacketHandler
	handshakeCrypto     handshake.Crypto
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	rekeyController *rekey.Controller,
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
				if spType == service.SessionReset {
					return fmt.Errorf("server requested cryptographyService reset")
				}
			}

			epoch := binary.BigEndian.Uint16(buffer[:2])
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
					fmt.Printf("rekey ack: derived new keys (client) c2s:%x s2c:%x\n", newC2S, newS2C)
					epoch, err := t.rekeyController.Crypto.Rekey(newC2S, newS2C)
					if err != nil {
						fmt.Printf("rekey ack: install new session failed: %v\n", err)
						continue
					}
					if err := t.rekeyController.ApplyKeys(newC2S, newS2C, uint16(epoch)); err != nil {
						fmt.Printf("rekey ack: apply keys failed: %v\n", err)
						continue
					}
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
				t.rekeyController.ConfirmSendEpoch(epoch)
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
