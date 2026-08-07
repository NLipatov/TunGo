package udp_chacha20

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing/transport"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

type transportRekeyController interface {
	ObservePeerEpoch(epoch uint16)
	ActivateSendEpoch(epoch uint16)
}

type rekeyAckHandler interface {
	HandleRekeyAck(carrierEpoch uint16, plaindata []byte) (ok bool, err error)
}

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService connection.Crypto
	rekeyController     transportRekeyController
	rekeyAck            rekeyAckHandler
	egress              connection.Egress
	lastRecvAt          time.Time
	lastPingSentAt      time.Time
	pingBuf             []byte
}

func NewTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService connection.Crypto,
	rekeyController transportRekeyController,
	rekeyAck rekeyAckHandler,
	egress connection.Egress,
) transport.Handler {
	const pingLen = udpPayloadOffset + 3
	return &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekeyController:     rekeyController,
		rekeyAck:            rekeyAck,
		egress:              egress,
		lastRecvAt:          time.Now(),
		pingBuf:             make([]byte, pingLen, pingLen+chacha20poly1305.Overhead),
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
					if err := t.handleIdle(); err != nil {
						return err
					}
					continue
				}
				if t.ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("could not read a packet from adapter: %v", readErr)
			}
			_, err := t.handleDatagram(buffer[:n])
			if err != nil {
				return err
			}
		}
	}
}

func (t *TransportHandler) handleDatagram(pkt []byte) (int, error) {
	if len(pkt) < 2 {
		return 0, nil
	}

	decrypted, decryptionErr := t.cryptographyService.Decrypt(pkt)
	if decryptionErr != nil {
		// Drop undecryptable packets without terminating session.
		// If session is truly broken, keepalive timeout will detect it.
		// This makes client resilient to packet corruption and garbage injection.
		return 0, nil
	}
	t.lastRecvAt = time.Now()
	var carrierEpoch uint16
	if len(pkt) >= udp.EpochOffset+2 {
		carrierEpoch = binary.BigEndian.Uint16(pkt[udp.EpochOffset : udp.EpochOffset+2])
	}

	if t.rekeyController != nil {
		// Authentication proves the peer epoch; UDP uses the same event to promote send.
		t.rekeyController.ObservePeerEpoch(carrierEpoch)
		t.rekeyController.ActivateSendEpoch(carrierEpoch)
	}

	if handled, err := t.handleControlplane(carrierEpoch, decrypted); handled {
		return 0, err
	}

	_, writeErr := t.writer.Write(decrypted)
	if writeErr != nil {
		if t.ctx.Err() != nil {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to write to TUN: %s", writeErr)
	}
	return len(decrypted), nil
}

// ErrEpochExhausted is returned when server signals epoch exhaustion.
// Client should reconnect with a fresh handshake.
var ErrEpochExhausted = errors.New("epoch exhausted; reconnect required")

func (t *TransportHandler) handleControlplane(
	carrierEpoch uint16,
	plaintext []byte,
) (handled bool, err error) {
	spType, spOk := service_packet.TryParseHeader(plaintext)
	if !spOk {
		return false, nil
	}

	switch spType {
	case service_packet.EpochExhausted:
		// Server cannot create new epochs - reconnect immediately.
		slog.Warn("received EpochExhausted from server, initiating reconnect")
		return true, ErrEpochExhausted
	case service_packet.RekeyAck:
		if t.rekeyAck == nil {
			return true, nil
		}
		if _, err := t.rekeyAck.HandleRekeyAck(carrierEpoch, plaintext); err != nil {
			slog.Error("rekey ack install/apply failed", "err", err)
			if errors.Is(err, chacha20.ErrEpochExhausted) {
				return true, ErrEpochExhausted
			}
		}
		return true, nil
	default:
		// ignore unknown service_packet packets (including Pong — recv timer already reset above)
		return true, nil
	}
}

func (t *TransportHandler) handleIdle() error {
	if time.Since(t.lastRecvAt) > settings.PingRestartTimeout {
		return fmt.Errorf("server unreachable (no data for %s)", settings.PingRestartTimeout)
	}
	if t.egress != nil && time.Since(t.lastPingSentAt) > settings.PingInterval {
		t.sendPing()
	}
	return nil
}

func (t *TransportHandler) sendPing() {
	payload := t.pingBuf[udpPayloadOffset:]
	if _, err := service_packet.EncodeV1Header(service_packet.Ping, payload); err != nil {
		return
	}
	if err := t.egress.Send(t.pingBuf[:]); err != nil {
		return
	}
	t.lastPingSentAt = time.Now()
}
