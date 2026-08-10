package udp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20"
	"tungo/internal/protocol/chacha20/udp"
	"tungo/internal/protocol/servicepacket"

	"golang.org/x/crypto/chacha20poly1305"
)

type transportRekey interface {
	ObservePeerEpoch(epoch uint16)
	ActivateSendEpoch(epoch uint16)
	HandleRekeyAck(carrierEpoch uint16, plaindata []byte) (ok bool, err error)
}

type transportHandler struct {
	ctx                 context.Context
	reader              io.Reader
	writer              io.Writer
	cryptographyService crypto
	rekey               transportRekey
	egress              sender
	lastRecvAt          time.Time
	lastPingSentAt      time.Time
	pingBuf             []byte
}

func newTransportHandler(
	ctx context.Context,
	reader io.Reader,
	writer io.Writer,
	cryptographyService crypto,
	rekey transportRekey,
	egress sender,
) *transportHandler {
	const pingLen = udpPayloadOffset + 3
	return &transportHandler{
		ctx:                 ctx,
		reader:              reader,
		writer:              writer,
		cryptographyService: cryptographyService,
		rekey:               rekey,
		egress:              egress,
		lastRecvAt:          time.Now(),
		pingBuf:             make([]byte, pingLen, pingLen+chacha20poly1305.Overhead),
	}
}

func (t *transportHandler) HandleTransport() error {
	var buffer [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte

	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			n, readErr := t.reader.Read(buffer[:])
			if readErr != nil {
				if errors.Is(readErr, os.ErrDeadlineExceeded) {
					if err := t.checkLiveness(); err != nil {
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

func (t *transportHandler) handleDatagram(pkt []byte) (int, error) {
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

	if t.rekey != nil {
		// Authentication proves the peer epoch; UDP uses the same event to promote send.
		t.rekey.ObservePeerEpoch(carrierEpoch)
		t.rekey.ActivateSendEpoch(carrierEpoch)
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

// errEpochExhausted is returned when server signals epoch exhaustion.
// Client should reconnect with a fresh handshake.
var errEpochExhausted = errors.New("epoch exhausted; reconnect required")

func (t *transportHandler) handleControlplane(
	carrierEpoch uint16,
	plaintext []byte,
) (handled bool, err error) {
	spType, spOk := servicepacket.Parse(plaintext)
	if !spOk {
		return false, nil
	}

	switch spType {
	case servicepacket.EpochExhausted:
		// Server cannot create new epochs - reconnect immediately.
		slog.Warn("received EpochExhausted from server, initiating reconnect")
		return true, errEpochExhausted
	case servicepacket.RekeyAck, servicepacket.RekeyAckV2:
		if t.rekey == nil {
			return true, nil
		}
		if _, err := t.rekey.HandleRekeyAck(carrierEpoch, plaintext); err != nil {
			slog.Error("rekey ack install/apply failed", "err", err)
			if errors.Is(err, chacha20.ErrEpochExhausted) {
				return true, errEpochExhausted
			}
		}
		return true, nil
	default:
		// ignore unknown service_packet packets (including Pong — recv timer already reset above)
		return true, nil
	}
}

func (t *transportHandler) checkLiveness() error {
	if time.Since(t.lastRecvAt) > settings.PingRestartTimeout {
		return fmt.Errorf("server unreachable (no data for %s)", settings.PingRestartTimeout)
	}
	if t.egress != nil && time.Since(t.lastPingSentAt) > settings.PingInterval {
		t.sendPing()
	}
	return nil
}

func (t *transportHandler) sendPing() {
	payload := t.pingBuf[udpPayloadOffset:]
	if err := servicepacket.Encode(servicepacket.Ping, payload); err != nil {
		return
	}
	if err := t.egress.Send(t.pingBuf[:]); err != nil {
		return
	}
	t.lastPingSentAt = time.Now()
}
