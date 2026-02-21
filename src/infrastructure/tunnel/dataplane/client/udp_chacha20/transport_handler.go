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
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/infrastructure/tunnel/controlplane"

	"golang.org/x/crypto/chacha20poly1305"
)

type TransportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService connection.Crypto
	rekeyController     *rekey.StateMachine
	handshakeCrypto     primitives.KeyDeriver
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
	rekeyController *rekey.StateMachine,
	egress connection.Egress,
) transport.Handler {
	const pingLen = chacha20.UDPRouteIDLength + chacha20poly1305.NonceSize + 3
	return &TransportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekeyController:     rekeyController,
		handshakeCrypto:     &primitives.DefaultKeyDeriver{},
		egress:              egress,
		lastRecvAt:          time.Now(),
		pingBuf:             make([]byte, pingLen, pingLen+chacha20poly1305.Overhead),
	}
}

func (t *TransportHandler) HandleTransport() error {
	var buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	rec := trafficstats.NewRecorder()
	defer rec.Flush()

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
			writtenBytes, err := t.handleDatagram(buffer[:n])
			if err != nil {
				return err
			}
			rec.RecordRX(uint64(writtenBytes))
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

	if t.rekeyController != nil {
		if len(pkt) >= chacha20.UDPEpochOffset+2 {
			epoch := binary.BigEndian.Uint16(pkt[chacha20.UDPEpochOffset : chacha20.UDPEpochOffset+2])
			// Data was successfully decrypted with epoch; allow encrypt with this epoch by promoting.
			t.rekeyController.ActivateSendEpoch(epoch)
		}
		t.rekeyController.AbortPendingIfExpired(time.Now())
	}

	if handled, err := t.handleControlplane(decrypted); handled {
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

func (t *TransportHandler) handleControlplane(plaintext []byte) (handled bool, err error) {
	spType, spOk := service_packet.TryParseHeader(plaintext)
	if !spOk {
		return false, nil
	}

	switch spType {
	case service_packet.EpochExhausted:
		// Server cannot create new epochs - reconnect immediately.
		log.Printf("received EpochExhausted from server, initiating reconnect")
		return true, ErrEpochExhausted
	case service_packet.RekeyAck:
		if t.rekeyController != nil && t.rekeyController.LastRekeyEpoch >= 65000 {
			log.Printf("rekey ack: epoch exhausted, requesting session reset")
			return true, ErrEpochExhausted
		}
		if _, err := controlplane.ClientHandleRekeyAck(t.handshakeCrypto, t.rekeyController, plaintext); err != nil {
			log.Printf("rekey ack: install/apply failed: %v", err)
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
	payload := t.pingBuf[chacha20.UDPRouteIDLength+chacha20poly1305.NonceSize:]
	if _, err := service_packet.EncodeV1Header(service_packet.Ping, payload); err != nil {
		return
	}
	if err := t.egress.SendControl(t.pingBuf[:]); err != nil {
		return
	}
	t.lastPingSentAt = time.Now()
}
