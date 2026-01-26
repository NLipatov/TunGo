package udp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/curve25519"
)

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService connection.Crypto
	servicePacket       service.PacketHandler
	handshakeCrypto     handshake.Crypto
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	servicePacket service.PacketHandler,
) transport.Handler {
	return &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
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
					session, ok := t.cryptographyService.(*chacha20.DefaultUdpSession)
					if !ok {
						fmt.Println("rekey ack: unsupported crypto session type")
						continue
					}
					priv, ok := session.PendingRekeyPrivateKey()
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
					currentKey := session.ClientToServerKey()
					newKey, err := t.handshakeCrypto.DeriveKey(shared, currentKey, []byte("tungo-rekey-v1"))
					if err != nil {
						fmt.Printf("rekey ack: derive key failed: %v\n", err)
						continue
					}
					fmt.Printf("rekey ack: derived new key (client): %x\n", newKey)
					session.ClearPendingRekeyPrivateKey()
				case service.SessionReset:
					return fmt.Errorf("server requested cryptographyService reset")
				default:
					// ignore unknown service packets
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
