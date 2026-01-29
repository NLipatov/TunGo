package tcp_chacha20

import (
	"context"
	"io"
	"log"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
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
	var buffer [settings.DefaultEthernetMTU + settings.TCPChacha20Overhead]byte

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, readErr := t.reader.Read(buffer[:])
			if readErr != nil {
				if t.ctx.Err() != nil {
					return nil
				}
				log.Printf("read from TCP failed: %v", readErr)
				return readErr
			}

			if n < chacha20poly1305.Overhead || n > settings.DefaultEthernetMTU+settings.TCPChacha20Overhead {
				log.Printf("invalid ciphertext length: %d", n)
				continue
			}

			payload, payloadErr := t.cryptographyService.Decrypt(buffer[:n])
			if payloadErr != nil {
				log.Printf("failed to decrypt data: %s", payloadErr)
				return payloadErr
			}
			if spType, spOk := service_packet.TryParseHeader(payload); spOk {
				if spType == service_packet.RekeyAck {
					t.handleRekeyAck(payload)
					continue
				}
			}
			if _, writeErr := t.writer.Write(payload); writeErr != nil {
				log.Printf("failed to write to TUN: %v", writeErr)
				return writeErr
			}
		}
	}
}

func (t *TransportHandler) handleRekeyAck(payload []byte) {
	if t.rekeyController == nil {
		return
	}
	if len(payload) < service_packet.RekeyPacketLen {
		log.Printf("rekey ack too short: %d", len(payload))
		return
	}
	priv, ok := t.rekeyController.PendingRekeyPrivateKey()
	if !ok {
		log.Printf("rekey ack: no pending client private key")
		return
	}
	serverPub := payload[3 : 3+service_packet.RekeyPublicKeyLen]
	shared, err := curve25519.X25519(priv[:], serverPub)
	if err != nil {
		log.Printf("rekey ack: failed to compute shared secret: %v", err)
		return
	}
	currentC2S := t.rekeyController.CurrentClientToServerKey()
	currentS2C := t.rekeyController.CurrentServerToClientKey()
	newC2S, err := t.handshakeCrypto.DeriveKey(shared, currentC2S, []byte("tungo-rekey-c2s"))
	if err != nil {
		log.Printf("rekey ack: derive key failed: %v", err)
		return
	}
	newS2C, err := t.handshakeCrypto.DeriveKey(shared, currentS2C, []byte("tungo-rekey-s2c"))
	if err != nil {
		log.Printf("rekey ack: derive key failed: %v", err)
		return
	}
	if epoch, err := t.rekeyController.StartRekey(newC2S, newS2C); err == nil {
		// For TCP we can switch immediately.
		t.rekeyController.ActivateSendEpoch(epoch)
		t.rekeyController.ClearPendingRekeyPrivateKey()
	} else {
		log.Printf("rekey ack: install/apply failed: %v", err)
	}
}
