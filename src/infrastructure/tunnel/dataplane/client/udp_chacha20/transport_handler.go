package udp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/controlplane"
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
			if err := t.handleDatagram(buffer[:n]); err != nil {
				return err
			}
		}
	}
}

func (t *TransportHandler) handleDatagram(pkt []byte) error {
	// SessionReset is sent as a legacy, unencrypted control packet.
	if spType, spOk := service_packet.TryParseHeader(pkt); spOk {
		if t.rekeyController != nil {
			t.rekeyController.AbortPendingIfExpired(time.Now())
		}
		if spType == service_packet.SessionReset {
			return fmt.Errorf("server requested cryptographyService reset")
		}
	}
	if len(pkt) < 2 {
		return nil
	}

	decrypted, decryptionErr := t.cryptographyService.Decrypt(pkt)
	if decryptionErr != nil {
		if t.ctx.Err() != nil {
			return nil
		}
		// Duplicate nonce detected – this may indicate a network retransmission or a replay attack.
		// In either case, skip this packet.
		if errors.Is(decryptionErr, chacha20.ErrNonUniqueNonce) {
			return nil
		}
		return fmt.Errorf("failed to decrypt data: %s", decryptionErr)
	}

	if t.rekeyController != nil {
		epoch := binary.BigEndian.Uint16(pkt[:2])
		// Data was successfully decrypted with epoch; allow encrypt with this epoch by promoting.
		t.rekeyController.ActivateSendEpoch(epoch)
		t.rekeyController.AbortPendingIfExpired(time.Now())
	}

	if handled, err := t.handleControlplane(decrypted); handled {
		return err
	}

	_, writeErr := t.writer.Write(decrypted)
	if writeErr != nil {
		if t.ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("failed to write to TUN: %s", writeErr)
	}
	return nil
}

func (t *TransportHandler) handleControlplane(plaintext []byte) (handled bool, err error) {
	spType, spOk := service_packet.TryParseHeader(plaintext)
	if !spOk {
		return false, nil
	}

	switch spType {
	case service_packet.RekeyAck:
		if t.rekeyController != nil && t.rekeyController.LastRekeyEpoch >= 65000 {
			log.Printf("rekey ack: epoch exhausted, requesting session reset")
			return true, fmt.Errorf("epoch exhausted; reconnect required")
		}
		if _, err := controlplane.ClientHandleRekeyAck(t.handshakeCrypto, t.rekeyController, plaintext); err != nil {
			log.Printf("rekey ack: install/apply failed: %v", err)
		}
		return true, nil
	case service_packet.SessionReset:
		return true, fmt.Errorf("server requested cryptographyService reset")
	default:
		// ignore unknown service_packet packets
		return true, nil
	}
}
